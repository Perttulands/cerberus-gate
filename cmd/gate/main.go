package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Perttulands/polis-utils/brclient"

	"polis/gate/internal/bead"
	"polis/gate/internal/city"
	"polis/gate/internal/pipeline"
	"polis/gate/internal/review"
	"polis/gate/internal/verdict"
)

const defaultHistoryLimit = 20

var filterValueRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var historyBRClient = brclient.New()

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	os.Exit(run(ctx, os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return 0
	}

	cmd := args[0]
	if cmd == "check" {
		return runCheck(ctx, args[1:])
	}
	if cmd == "review" {
		return runReview(ctx, args[1:])
	}
	if cmd == "health" {
		return runHealth(ctx, args[1:])
	}
	if cmd == "city" {
		return runCity(ctx, args[1:])
	}
	if cmd == "history" {
		return runHistory(args[1:])
	}
	if cmd == "catalog-check" {
		return runCatalogCheck(ctx, args[1:])
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
	printUsage()
	return 1
}

func runCheck(ctx context.Context, args []string) int {
	var repoPath, level, citizen string
	var jsonOutput, notify bool

	level = pipeline.LevelStandard
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--level":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--level requires a value")
				return 1
			}
			level = args[i]
		case "--json":
			jsonOutput = true
		case "--notify":
			notify = true
		case "--citizen":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--citizen requires a value")
				return 1
			}
			citizen = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
				return 1
			}
			if repoPath == "" {
				repoPath = args[i]
			}
		}
		i++
	}

	if repoPath == "" {
		fmt.Fprintln(os.Stderr, "repo path required: gate check <repo-path>")
		return 1
	}

	if !pipeline.ValidLevel(level) {
		fmt.Fprintf(os.Stderr, "invalid level %q: use quick, standard, or deep\n", level)
		return 1
	}

	citizen = resolveCitizen(citizen, repoPath)

	v := pipeline.Run(ctx, repoPath, level, citizen)
	if (notify || strings.EqualFold(strings.TrimSpace(os.Getenv("CI")), "true")) && v.Score < 1.0 {
		publishFailureAlert(v)
	}

	if beadID := bead.Record(v); beadID != "" {
		v.Bead = beadID
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(v)
	} else {
		printPretty(v)
	}

	return v.ExitCode
}

func runReview(ctx context.Context, args []string) int {
	var beadID, repoPath, runtime string
	var jsonOutput, contextOnly bool

	repoPath = "."
	runtime = "codex"
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--bead":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--bead requires a value")
				return 1
			}
			beadID = args[i]
		case "--runtime":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--runtime requires a value")
				return 1
			}
			runtime = args[i]
		case "--json":
			jsonOutput = true
		case "--context-only":
			contextOnly = true
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
				return 1
			}
			if repoPath == "." {
				repoPath = args[i]
			}
		}
		i++
	}

	if beadID == "" {
		fmt.Fprintln(os.Stderr, "--bead flag required: gate review --bead <id> [repo-path]")
		return 1
	}

	if runtime != "codex" && runtime != "claude" {
		fmt.Fprintf(os.Stderr, "invalid runtime %q: use codex or claude\n", runtime)
		return 1
	}

	// Step 1: Assemble context bundle
	bundle, err := review.Assemble(ctx, beadID, repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
		return 1
	}

	// If fast-path failed, report immediately — no point spawning an agent
	if bundle.FastPath != nil && !bundle.FastPath.Pass {
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(bundle)
		} else {
			fmt.Print(review.FormatBundle(bundle))
			fmt.Fprintf(os.Stderr, "\nfast-path gate FAILED — skipping agent review\n")
		}
		return verdict.ExitFail
	}

	// Step 2: Write prompt file
	promptPath, err := review.WritePromptFile(bundle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
		return 1
	}

	// --context-only: print context bundle and prompt path, don't spawn
	if contextOnly {
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(bundle)
		} else {
			fmt.Print(review.FormatBundle(bundle))
			fmt.Printf("\nprompt written to: %s\n", promptPath)
			fmt.Printf("verdict expected at: %s\n", review.VerdictPath(beadID))
		}
		return verdict.ExitPass
	}

	// Step 3: Spawn agent and wait for verdict
	sessionName := fmt.Sprintf("gate-review-%s", beadID)
	verdictPath := review.VerdictPath(beadID)

	if !jsonOutput {
		fmt.Printf("spawning %s agent in tmux session %q...\n", runtime, sessionName)
	}

	spawnResult, err := review.SpawnAndWait(review.SpawnConfig{
		SessionName: sessionName,
		WorkDir:     repoPath,
		PromptPath:  promptPath,
		VerdictPath: verdictPath,
		Runtime:     runtime,
		Deadline:    10 * time.Minute,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn failed: %v\n", err)
		return 1
	}

	if spawnResult.TimedOut {
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(map[string]any{
				"bead_id": beadID,
				"verdict": "UNCLEAR",
				"summary": fmt.Sprintf("agent timed out after %v — manual review required", spawnResult.Duration),
			})
		} else {
			fmt.Fprintf(os.Stderr, "\nagent timed out after %v — manual review required\n", spawnResult.Duration)
		}
		return verdict.ExitReview
	}

	// Verdict file received — parse and validate
	verdictData, err := os.ReadFile(verdictPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read verdict: %v\n", err)
		return 1
	}

	rv, err := review.ParseVerdict(verdictData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid verdict: %v\n", err)
		if !jsonOutput {
			fmt.Fprintf(os.Stderr, "raw verdict:\n%s\n", string(verdictData))
		}
		return 1
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rv)
	} else {
		fmt.Print(review.FormatPrettyVerdict(rv))
	}

	return review.ExitCodeForVerdict(rv)
}

func runHealth(ctx context.Context, args []string) int {
	var repoPath, citizen string
	var jsonOutput bool

	repoPath = "."
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--citizen":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--citizen requires a value")
				return 1
			}
			citizen = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
				return 1
			}
			if repoPath == "." {
				repoPath = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "unexpected extra argument: %s\n", args[i])
				return 1
			}
		}
		i++
	}

	citizen = resolveCitizen(citizen, repoPath)
	v := pipeline.Run(ctx, repoPath, pipeline.LevelQuick, citizen)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(v)
		return v.ExitCode
	}

	if v.Pass {
		fmt.Fprintln(os.Stdout, "healthy")
		return verdict.ExitPass
	}

	var failed []string
	for _, g := range v.Gates {
		if !g.Pass && !g.Skipped {
			failed = append(failed, g.Name)
		}
	}
	if len(failed) == 0 {
		fmt.Fprintln(os.Stderr, "unhealthy")
	} else {
		fmt.Fprintf(os.Stderr, "unhealthy: %s\n", strings.Join(failed, ", "))
	}
	return v.ExitCode
}

func runCity(ctx context.Context, args []string) int {
	var repoPath, installAt, citizen string
	var jsonOutput, skipStandalone bool
	standaloneTimeout := 120 * time.Second

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--install-at":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--install-at requires a value")
				return city.ExitInvalid
			}
			installAt = args[i]
		case "--skip-standalone":
			skipStandalone = true
		case "--standalone-timeout":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--standalone-timeout requires a value")
				return city.ExitInvalid
			}
			d, err := time.ParseDuration(args[i])
			if err != nil || d <= 0 {
				fmt.Fprintf(os.Stderr, "invalid --standalone-timeout %q: use duration like 120s\n", args[i])
				return city.ExitInvalid
			}
			standaloneTimeout = d
		case "--json":
			jsonOutput = true
		case "--citizen":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--citizen requires a value")
				return city.ExitInvalid
			}
			citizen = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
				return city.ExitInvalid
			}
			if repoPath == "" {
				repoPath = args[i]
			}
		}
		i++
	}

	if repoPath == "" {
		fmt.Fprintln(os.Stderr, "repo path required: gate city <repo-path>")
		return city.ExitInvalid
	}

	citizen = resolveCitizen(citizen, repoPath)

	v := city.Run(ctx, repoPath, city.Options{
		InstallAt:         installAt,
		SkipStandalone:    skipStandalone,
		StandaloneTimeout: standaloneTimeout,
	})
	if beadID := bead.RecordCity(v, citizen); beadID != "" {
		v.Bead = beadID
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(v)
	} else {
		printPrettyCity(v)
	}
	return v.ExitCode
}

func runHistory(args []string) int {
	if !historyBRClient.Available() {
		fmt.Fprintln(os.Stderr, "gate history requires br (beads) to be installed")
		return 1
	}

	var repoFilter, assigneeFilter string
	limit := defaultHistoryLimit
	i := 0
	for i < len(args) {
		switch args[i] {
		case "--repo":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--repo requires a value")
				return 1
			}
			v, err := validateFilterValue("--repo", args[i])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			repoFilter = v
		case "--citizen":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--citizen requires a value")
				return 1
			}
			v, err := validateFilterValue("--citizen", args[i])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			assigneeFilter = v
		case "--limit":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--limit requires a value")
				return 1
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "--limit must be a positive integer")
				return 1
			}
			limit = n
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
				return 1
			}
		}
		i++
	}

	brArgs := []string{"search", "gate", "--type", "gate", "--sort", "created", "--reverse", "--limit", strconv.Itoa(limit)}
	if repoFilter != "" {
		brArgs = append(brArgs, "--label", "repo:"+repoFilter)
	}
	if assigneeFilter != "" {
		brArgs = append(brArgs, "--assignee", assigneeFilter)
	}

	result, err := historyBRClient.Run(context.Background(), brclient.Invocation{Args: brArgs})
	if len(result.Stdout) > 0 {
		os.Stdout.Write(result.Stdout)
	}
	if len(result.Stderr) > 0 {
		os.Stderr.Write(result.Stderr)
	}
	if err != nil {
		var cmdErr *brclient.CommandError
		if errors.As(err, &cmdErr) && cmdErr.Result.ExitCode > 0 {
			return cmdErr.Result.ExitCode
		}
		fmt.Fprintf(os.Stderr, "br search failed: %v\n", err)
		return 1
	}
	return 0
}

func resolveCitizen(explicit, repoPath string) string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit
	}
	if envVal, ok := os.LookupEnv("POLIS_CITIZEN"); ok {
		envVal = strings.TrimSpace(envVal)
		if envVal != "" {
			return envVal
		}
	}
	if gitName := gitUserName(repoPath); gitName != "" {
		return gitName
	}
	return "unknown"
}

func validateFilterValue(flagName, raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("%s value cannot be empty", flagName)
	}
	if !filterValueRe.MatchString(v) {
		return "", fmt.Errorf("%s must match %s", flagName, filterValueRe.String())
	}
	return v, nil
}

func printUsage() {
	fmt.Println(`gate — quality gate for Polis

Usage:
  gate check <repo-path> [flags]
  gate review --bead <id> [repo-path] [flags]
  gate health [repo-path] [flags]
  gate city <repo-path> [flags]
  gate catalog-check [flags]
  gate history [flags]

Review flags:
  --bead <id>                   Bead ID to review (required)
  --runtime codex|claude        Agent runtime (default: codex)
  --context-only                Assemble context only, don't spawn agent
  --json                        Output as JSON
  [repo-path]                   Repo to review (default: .)

Check flags:
  --level quick|standard|deep   Check level (default: standard)
  --json                        Output verdict as JSON
  --notify                      Publish relay alert on failure
  --citizen <name>              Set actor name

Health flags:
  [repo-path]                   Repo to probe (default: .)
  --json                        Output quick verdict as JSON
  --citizen <name>              Set actor name

City flags:
  --install-at <path>           Also run split check against install path
  --skip-standalone             Skip standalone check (status=skip)
  --standalone-timeout <dur>    Timeout for standalone_check (default: 120s)
  --json                        Output verdict as JSON
  --citizen <name>              Set actor name

Catalog-check flags:
  --registry <path>             Registry file (default: /home/polis/tools/gate/registry.yaml)
  --json                        Output results as JSON

History flags:
  --repo <name>                 Filter by repo name
  --citizen <name>              Filter by citizen
  --limit N                     Max results (default: 20)`)
}

func printPretty(v verdict.Verdict) {
	icon := "\033[32m✓ PASS\033[0m"
	if !v.Pass {
		icon = "\033[31m✗ FAIL\033[0m"
	}
	fmt.Printf("\n%s  %s @ %s level  (score: %.2f)\n", icon, v.Repo, v.Level, v.Score)
	fmt.Printf("citizen: %s\n\n", v.Citizen)

	for _, g := range v.Gates {
		gIcon := "\033[32m✓\033[0m"
		if g.Skipped {
			gIcon = "\033[33m-\033[0m"
		} else if !g.Pass {
			gIcon = "\033[31m✗\033[0m"
		}
		fmt.Printf("  %s %-20s %dms\n", gIcon, g.Name, g.DurationMs)
		if !g.Pass && !g.Skipped && g.Output != "" {
			for _, line := range strings.Split(g.Output, "\n") {
				if line != "" {
					fmt.Printf("    %s\n", line)
				}
			}
		}
	}
	if v.Bead != "" {
		fmt.Printf("\nbead: %s\n", v.Bead)
	}
	fmt.Println()
}

func printPrettyCity(v city.Verdict) {
	color := "\033[32m✓ PASS\033[0m"
	if v.Status == "warn" {
		color = "\033[33m! WARN\033[0m"
	}
	if v.Status == "fail" {
		color = "\033[31m✗ FAIL\033[0m"
	}
	fmt.Printf("\n%s  %s (city)\n\n", color, v.Repo)

	for _, c := range v.Checks {
		icon := "\033[32m✓\033[0m"
		if c.Status == city.StatusSkip {
			icon = "\033[33m-\033[0m"
		} else if c.Status == city.StatusFail {
			icon = "\033[31m✗\033[0m"
		}
		fmt.Printf("  %s %-12s %dms  %s\n", icon, c.Name, c.DurationMs, c.Detail)
	}

	fmt.Printf("\nsummary: pass=%d fail=%d skip=%d\n", v.Summary.Pass, v.Summary.Fail, v.Summary.Skip)
	if v.Bead != "" {
		fmt.Printf("bead: %s\n", v.Bead)
	}
	fmt.Println()
}

func gitUserName(repoPath string) string {
	cmd := exec.Command("git", "config", "user.name")
	if strings.TrimSpace(repoPath) != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func publishFailureAlert(v verdict.Verdict) {
	body, err := buildFailureAlertBody(v)
	if err != nil {
		return
	}
	exec.Command("relay", "send", "--to", "system", "--type", "alert", "--body", string(body)).Run()
}

func buildFailureAlertBody(v verdict.Verdict) ([]byte, error) {
	failed := make([]string, 0, len(v.Gates))
	for _, gate := range v.Gates {
		if !gate.Pass && !gate.Skipped {
			failed = append(failed, gate.Name)
		}
	}
	return json.Marshal(map[string]any{
		"source":       "gate",
		"repo":         v.Repo,
		"score":        v.Score,
		"failed_gates": failed,
		"severity":     "warning",
	})
}
