package gates

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"polis/gate/internal/verdict"
)

// ubsReport models the JSON output of `ubs --format=json`.
type ubsReport struct {
	Scanners []struct {
		Critical int    `json:"critical"`
		Warning  int    `json:"warning"`
		Info     int    `json:"info"`
		Language string `json:"language"`
	} `json:"scanners"`
	Totals struct {
		Critical int `json:"critical"`
		Warning  int `json:"warning"`
		Info     int `json:"info"`
		Files    int `json:"files"`
	} `json:"totals"`
}

// RunUBS runs ubs build health check on the repo at dir.
// UBS is optional — if not installed, the gate passes with skipped=true.
// Pass criteria: no critical-level failures in output.
func RunUBS(ctx context.Context, dir string, timeoutSec int) verdict.GateResult {
	return runUBS(ctx, dir, timeoutSec, false, nil)
}

// RunUBSDiff runs ubs in diff mode (changed files only).
func RunUBSDiff(ctx context.Context, dir string, timeoutSec int, cfg *Config) verdict.GateResult {
	return runUBS(ctx, dir, timeoutSec, true, cfg)
}

func runUBS(ctx context.Context, dir string, timeoutSec int, diffMode bool, cfg *Config) verdict.GateResult {
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	start := time.Now()
	args := []string{"--format=json", "."}
	if diffMode {
		args = []string{"--diff", "--format=json", "."}
	}
	if cfg != nil {
		if diffMode && len(cfg.Check.UBSDiff) > 0 {
			args = cfg.Check.UBSDiff
		} else if !diffMode && len(cfg.Check.UBS) > 0 {
			args = cfg.Check.UBS
		}
	}
	cmdPass, output, err := runCmd(ctx, dir, timeoutSec, "ubs", args...)
	if diffMode && err == nil && !cmdPass {
		// Diff mode can fail in non-git contexts; fall back to full scan.
		cmdPass, output, err = runCmd(ctx, dir, timeoutSec, "ubs", "--format=json", ".")
	}
	dur := time.Since(start).Milliseconds()

	if err != nil {
		return verdict.GateResult{
			Name:       "ubs",
			Pass:       true,
			Skipped:    true,
			Output:     "ubs not available (skipped)",
			DurationMs: dur,
		}
	}

	findings := parseUBSOutput(output)
	pass := cmdPass && findings.Errors == 0

	summary := fmt.Sprintf("critical=%d warning=%d info=%d", findings.Errors, findings.Warnings, findings.Info)
	if !pass {
		summary = fmt.Sprintf("critical=%d warning=%d info=%d (cmd_pass=%v)", findings.Errors, findings.Warnings, findings.Info, cmdPass)
	}

	return verdict.GateResult{
		Name:       "ubs",
		Pass:       pass,
		Output:     summary,
		DurationMs: dur,
		Findings:   &findings,
	}
}

// parseUBSOutput extracts finding counts from UBS JSON output.
// It uses json.Decoder to robustly locate the JSON object even when the
// output is prefixed by non-JSON banner/log lines. Falls back to counting
// icon prefixes in plain-text output if no valid JSON is found.
func parseUBSOutput(output string) verdict.Findings {
	var f verdict.Findings
	raw := strings.TrimSpace(output)
	if raw == "" {
		return f
	}

	// Locate the start of the JSON object. UBS emits banner lines
	// (e.g. "UBS Meta-Runner v5.0.7 ...") before the JSON blob.
	if idx := strings.Index(raw, "{"); idx >= 0 {
		var report ubsReport
		dec := json.NewDecoder(strings.NewReader(raw[idx:]))
		if err := dec.Decode(&report); err == nil {
			// Prefer the totals when present.
			if report.Totals.Critical > 0 || report.Totals.Warning > 0 || report.Totals.Info > 0 || report.Totals.Files > 0 {
				return verdict.Findings{
					Errors:   report.Totals.Critical,
					Warnings: report.Totals.Warning,
					Info:     report.Totals.Info,
				}
			}
			// Totals are all zeros; cross-check by summing per-scanner counts.
			if len(report.Scanners) > 0 {
				for _, s := range report.Scanners {
					f.Errors += s.Critical
					f.Warnings += s.Warning
					f.Info += s.Info
				}
				return f
			}
			// Valid JSON with zero totals and no scanners — clean scan.
			return verdict.Findings{
				Errors:   report.Totals.Critical,
				Warnings: report.Totals.Warning,
				Info:     report.Totals.Info,
			}
		}
	}

	// Fallback: count icon prefixes in plain-text output.
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "\u2717") {
			f.Errors++
		} else if strings.HasPrefix(trimmed, "\u26a0") {
			f.Warnings++
		}
	}
	return f
}
