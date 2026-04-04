package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Perttulands/polis-utils/brclient"

	"polis/gate/internal/pipeline"
	"polis/gate/internal/verdict"
)

// BeadInfo holds the parsed bead metadata from br show --json.
type BeadInfo struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	Labels      []string `json:"labels"`
	Assignee    string   `json:"assignee"`
	Project     string   `json:"project"`
}

// ContextBundle is the assembled review context for a bead.
type ContextBundle struct {
	Bead     *BeadInfo       `json:"bead"`
	Diff     string          `json:"diff"`
	FastPath *verdict.Verdict `json:"fast_path"`
	Repo     string          `json:"repo"`
	Branch   string          `json:"branch"`
}

var brClient = brclient.New()

// FetchBead retrieves bead metadata via br show --json.
func FetchBead(beadID string) (*BeadInfo, error) {
	if !brClient.Available() {
		return nil, fmt.Errorf("br binary not found on PATH")
	}

	result, err := brClient.Run(context.Background(), brclient.Invocation{
		Args: []string{"show", beadID, "--json"},
	})
	if err != nil {
		stderr := strings.TrimSpace(string(result.Stderr))
		if stderr != "" {
			return nil, fmt.Errorf("br show failed: %s", stderr)
		}
		return nil, fmt.Errorf("br show failed: %w", err)
	}

	var bead BeadInfo
	if err := json.Unmarshal(result.Stdout, &bead); err != nil {
		return nil, fmt.Errorf("failed to parse bead JSON: %w", err)
	}
	return &bead, nil
}

// FetchDiff returns the git diff for the repo (staged + unstaged vs HEAD).
func FetchDiff(repoPath string) (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		// If HEAD doesn't exist (empty repo), try diff without HEAD
		cmd2 := exec.Command("git", "diff")
		cmd2.Dir = repoPath
		out, err = cmd2.Output()
		if err != nil {
			return "", fmt.Errorf("git diff failed: %w", err)
		}
	}
	return string(out), nil
}

// FetchBranch returns the current git branch name.
func FetchBranch(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// RunFastPath runs the gate check pipeline and returns the verdict.
func RunFastPath(ctx context.Context, repoPath string) verdict.Verdict {
	return pipeline.Run(ctx, repoPath, pipeline.LevelStandard, "gate-review")
}

// Assemble builds the full context bundle for a review.
func Assemble(ctx context.Context, beadID, repoPath string) (*ContextBundle, error) {
	bead, err := FetchBead(beadID)
	if err != nil {
		return nil, fmt.Errorf("fetch bead: %w", err)
	}

	diff, err := FetchDiff(repoPath)
	if err != nil {
		return nil, fmt.Errorf("fetch diff: %w", err)
	}

	fastPath := RunFastPath(ctx, repoPath)
	branch := FetchBranch(repoPath)

	return &ContextBundle{
		Bead:     bead,
		Diff:     diff,
		FastPath: &fastPath,
		Repo:     repoPath,
		Branch:   branch,
	}, nil
}

// FormatBundle produces a human-readable context bundle for stdout.
func FormatBundle(b *ContextBundle) string {
	var sb strings.Builder

	sb.WriteString("BEAD:\n")
	sb.WriteString(fmt.Sprintf("  id: %s\n", b.Bead.ID))
	sb.WriteString(fmt.Sprintf("  title: %s\n", b.Bead.Title))
	if b.Bead.Description != "" {
		sb.WriteString(fmt.Sprintf("  description: |\n"))
		for _, line := range strings.Split(b.Bead.Description, "\n") {
			sb.WriteString(fmt.Sprintf("    %s\n", line))
		}
	}
	sb.WriteString(fmt.Sprintf("  status: %s\n", b.Bead.Status))
	sb.WriteString(fmt.Sprintf("  priority: P%d\n", b.Bead.Priority))
	if len(b.Bead.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("  labels: %s\n", strings.Join(b.Bead.Labels, ", ")))
	}

	sb.WriteString("\nREPO:\n")
	sb.WriteString(fmt.Sprintf("  path: %s\n", b.Repo))
	sb.WriteString(fmt.Sprintf("  branch: %s\n", b.Branch))

	sb.WriteString("\nFAST-PATH:\n")
	if b.FastPath != nil {
		scoreStr := "PASS"
		if !b.FastPath.Pass {
			scoreStr = "FAIL"
		}
		sb.WriteString(fmt.Sprintf("  gate-score: %.2f (%s)\n", b.FastPath.Score, scoreStr))
		for _, g := range b.FastPath.Gates {
			status := "PASS"
			if g.Skipped {
				status = "SKIP"
			} else if !g.Pass {
				status = "FAIL"
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", g.Name, status))
		}
	}

	sb.WriteString("\nDIFF:\n")
	if strings.TrimSpace(b.Diff) == "" {
		sb.WriteString("  (no changes)\n")
	} else {
		for _, line := range strings.Split(b.Diff, "\n") {
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}

	return sb.String()
}

// VerdictSchema is the JSON schema agents must write their verdict to.
const VerdictSchema = `{
  "bead_id": "<string>",
  "verdict": "PASS | PARTIAL | FAIL | UNCLEAR",
  "scope_items": [
    {
      "item": "<scope claim from done-condition>",
      "status": "PASS | FAIL | UNCLEAR",
      "evidence": "<what you checked and found>",
      "suggestion": "<optional — what to fix>"
    }
  ],
  "gate_check_consistent": true,
  "summary": "<1-2 sentence overall assessment>"
}`

// promptTemplate is the Cerberus review prompt from the gate-agent spec (section 7).
const promptTemplate = `You are Cerberus, the gate agent for Polis. You are reviewing whether a code
change satisfies a bead's done-condition.

## Bead
id: %s
title: %s

%s

## Done Condition
%s

## Diff
%s

## Fast-Path Gate Results
%s

## Your Task
1. Read the done-condition and identify every distinct verifiable claim.
2. For each claim, determine how to verify it — examine the diff, read files,
   run commands, check system state.
3. Execute the verification. Do not guess — check.
4. Write your verdict to %s in this exact JSON schema:
%s

## Rules
- If a claim requires checking system state (like "br list shows only real
  work items"), actually run the command.
- If a claim requires checking file contents, read the file.
- If the diff addresses the claim but you're not 100%% certain, mark PASS with
  a note about what you checked.
- If you genuinely cannot determine status, use UNCLEAR — do not guess.
- Do not modify any files. Do not commit. Do not close beads. Read-only.
`

// PromptPath returns the path where the review prompt file will be written.
func PromptPath(beadID string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("gate-review-%s.md", beadID))
}

// VerdictPath returns the path where the agent writes its verdict.
func VerdictPath(beadID string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("gate-review-%s-verdict.json", beadID))
}

// formatFastPathForPrompt produces a compact fast-path summary for the prompt.
func formatFastPathForPrompt(fp *verdict.Verdict) string {
	if fp == nil {
		return "(not run)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("gate-score: %.2f\n", fp.Score))
	for _, g := range fp.Gates {
		status := "PASS"
		if g.Skipped {
			status = "SKIP"
		} else if !g.Pass {
			status = "FAIL"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", g.Name, status))
	}
	return sb.String()
}

// formatDescriptionBlock returns the full bead body for the prompt.
func formatDescriptionBlock(b *BeadInfo) string {
	if b.Description == "" {
		return "(no description)"
	}
	return b.Description
}

// extractDoneCondition extracts a done-condition from the bead description.
// Looks for "Done when:" or "DONE WHEN:" prefix. Falls back to full description.
func extractDoneCondition(b *BeadInfo) string {
	desc := b.Description
	if desc == "" {
		return b.Title
	}
	lower := strings.ToLower(desc)
	for _, marker := range []string{"done when:", "done-condition:"} {
		idx := strings.Index(lower, marker)
		if idx >= 0 {
			return strings.TrimSpace(desc[idx+len(marker):])
		}
	}
	return desc
}

// RenderPrompt formats the review prompt from a context bundle.
func RenderPrompt(bundle *ContextBundle) string {
	verdictPath := VerdictPath(bundle.Bead.ID)
	return fmt.Sprintf(promptTemplate,
		bundle.Bead.ID,
		bundle.Bead.Title,
		formatDescriptionBlock(bundle.Bead),
		extractDoneCondition(bundle.Bead),
		bundle.Diff,
		formatFastPathForPrompt(bundle.FastPath),
		verdictPath,
		VerdictSchema,
	)
}

// WritePromptFile writes the rendered review prompt to /tmp/gate-review-<id>.md.
func WritePromptFile(bundle *ContextBundle) (string, error) {
	prompt := RenderPrompt(bundle)
	path := PromptPath(bundle.Bead.ID)
	if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
		return "", fmt.Errorf("write prompt file: %w", err)
	}
	return path, nil
}
