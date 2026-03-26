package review

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SpawnConfig controls how the review agent is spawned.
type SpawnConfig struct {
	SessionName string
	WorkDir     string
	PromptPath  string
	VerdictPath string
	Runtime     string        // "codex" or "claude"
	Deadline    time.Duration // default: 10 minutes
}

// SpawnResult holds the outcome of a spawn+wait cycle.
type SpawnResult struct {
	SessionName string
	VerdictPath string
	TimedOut    bool
	Duration    time.Duration
}

// runtimeCommand returns the CLI command and args for a given runtime.
func runtimeCommand(runtime string) (string, []string) {
	switch runtime {
	case "claude":
		return "claude", []string{"--dangerously-skip-permissions"}
	default: // codex
		return "codex", []string{"--ask-for-approval", "never", "--sandbox", "danger-full-access"}
	}
}

// SpawnAndWait creates a tmux session, launches the runtime, sends the prompt,
// and waits for the verdict file to appear. Returns when verdict arrives or deadline expires.
func SpawnAndWait(cfg SpawnConfig) (*SpawnResult, error) {
	if cfg.Deadline == 0 {
		cfg.Deadline = 10 * time.Minute
	}
	if cfg.Runtime == "" {
		cfg.Runtime = "codex"
	}

	start := time.Now()
	result := &SpawnResult{
		SessionName: cfg.SessionName,
		VerdictPath: cfg.VerdictPath,
	}

	// 1. Check tmux available
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux not found on PATH")
	}

	// 2. Create detached tmux session
	createCmd := exec.Command("tmux", "new-session", "-d", "-s", cfg.SessionName, "-c", cfg.WorkDir)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("create tmux session: %s: %s", err, out)
	}

	// 3. Unset parent env vars to prevent recursive agent loops
	sendKeys(cfg.SessionName, "unset CLAUDECODE CLAUDE_CODE_ENTRYPOINT ANTHROPIC_API_KEY_PARENT")
	time.Sleep(500 * time.Millisecond)

	// 4. Launch runtime
	bin, args := runtimeCommand(cfg.Runtime)
	launchCmd := bin
	if len(args) > 0 {
		launchCmd = bin + " " + strings.Join(args, " ")
	}
	sendKeys(cfg.SessionName, launchCmd)

	// 5. Wait for runtime to be ready (up to 60s)
	if err := waitForReady(cfg.SessionName, cfg.Runtime, 60*time.Second); err != nil {
		killSession(cfg.SessionName)
		return nil, fmt.Errorf("runtime not ready: %w", err)
	}

	// 6. Send the review prompt via tmux load-buffer + paste-buffer
	if err := sendPromptFile(cfg.SessionName, cfg.PromptPath); err != nil {
		killSession(cfg.SessionName)
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	// 7. Wait for verdict file with deadline
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.After(cfg.Deadline)

	for {
		select {
		case <-deadline:
			result.TimedOut = true
			result.Duration = time.Since(start)
			killSession(cfg.SessionName)
			return result, nil
		case <-ticker.C:
			if _, err := os.Stat(cfg.VerdictPath); err == nil {
				result.Duration = time.Since(start)
				return result, nil
			}
		}
	}
}

// sendKeys sends literal text to a tmux session and presses ENTER.
func sendKeys(session, text string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", session, "-l", text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys: %s: %s", err, out)
	}
	time.Sleep(200 * time.Millisecond)
	// Send ENTER
	enterCmd := exec.Command("tmux", "send-keys", "-t", session, "ENTER")
	if out, err := enterCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send ENTER: %s: %s", err, out)
	}
	return nil
}

// sendPromptFile loads a prompt file into tmux buffer and pastes it.
func sendPromptFile(session, promptPath string) error {
	// Load file into tmux buffer
	loadCmd := exec.Command("tmux", "load-buffer", promptPath)
	if out, err := loadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("load-buffer: %s: %s", err, out)
	}

	// Paste buffer into session
	pasteCmd := exec.Command("tmux", "paste-buffer", "-t", session)
	if out, err := pasteCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("paste-buffer: %s: %s", err, out)
	}

	// Wait for TUI to process, then send ENTER twice
	time.Sleep(200 * time.Millisecond)
	if out, err := exec.Command("tmux", "send-keys", "-t", session, "ENTER").CombinedOutput(); err != nil {
		return fmt.Errorf("send ENTER after paste: %s: %s", err, out)
	}
	time.Sleep(100 * time.Millisecond)
	if out, err := exec.Command("tmux", "send-keys", "-t", session, "ENTER").CombinedOutput(); err != nil {
		return fmt.Errorf("send second ENTER after paste: %s: %s", err, out)
	}

	return nil
}

// waitForReady polls the tmux pane for readiness indicators.
func waitForReady(session, runtime string, timeout time.Duration) error {
	readyPatterns := []string{"❯"}
	switch runtime {
	case "codex":
		readyPatterns = append(readyPatterns, "OpenAI Codex", ">_ OpenAI Codex")
	case "claude":
		readyPatterns = append(readyPatterns, "Claude Code v")
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("%s did not become ready within %v", runtime, timeout)
		case <-ticker.C:
			content := capturePane(session)
			for _, pattern := range readyPatterns {
				if strings.Contains(content, pattern) {
					// Check for trust dialog and auto-approve
					if strings.Contains(content, "trust this folder") {
						if _, err := exec.Command("tmux", "send-keys", "-t", session, "ENTER").CombinedOutput(); err != nil {
						fmt.Fprintf(os.Stderr, "warning: trust dialog ENTER failed: %v\n", err)
					}
						time.Sleep(2 * time.Second)
					}
					return nil
				}
			}
		}
	}
}

// capturePane returns the visible content of a tmux pane.
func capturePane(session string) string {
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// killSession terminates a tmux session.
func killSession(session string) {
	exec.Command("tmux", "kill-session", "-t", session).Run()
}
