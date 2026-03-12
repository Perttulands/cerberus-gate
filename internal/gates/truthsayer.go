package gates

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"polis/gate/internal/verdict"
)

// truthsayerReport models the JSON output of `truthsayer scan --format json`.
type truthsayerReport struct {
	Findings []struct {
		Severity string `json:"severity"`
	} `json:"findings"`
	Summary struct {
		Errors   int `json:"errors"`
		Warnings int `json:"warnings"`
		Info     int `json:"info"`
	} `json:"summary"`
}

// RunTruthsayer runs truthsayer scan on the repo at dir.
// Truthsayer is optional — if not installed, the gate passes with skipped=true.
// Pass criteria: zero critical (error) findings.
func RunTruthsayer(ctx context.Context, dir string, timeoutSec int) verdict.GateResult {
	return runTruthsayer(ctx, dir, timeoutSec, false, nil)
}

// RunTruthsayerCI runs truthsayer in CI mode (changed lines/files focus).
func RunTruthsayerCI(ctx context.Context, dir string, timeoutSec int, cfg *Config) verdict.GateResult {
	return runTruthsayer(ctx, dir, timeoutSec, true, cfg)
}

func runTruthsayer(ctx context.Context, dir string, timeoutSec int, ciMode bool, cfg *Config) verdict.GateResult {
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	start := time.Now()
	args := []string{"scan", ".", "--format", "json"}
	if ciMode {
		args = []string{"ci", "."}
	}
	if cfg != nil {
		if ciMode && len(cfg.Check.TruthsayerCI) > 0 {
			args = cfg.Check.TruthsayerCI
		} else if !ciMode && len(cfg.Check.Truthsayer) > 0 {
			args = cfg.Check.Truthsayer
		}
	}
	cmdPass, output, err := runCmd(ctx, dir, timeoutSec, "truthsayer", args...)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		return verdict.GateResult{
			Name:       "truthsayer",
			Pass:       true,
			Skipped:    true,
			Output:     "truthsayer not available (skipped)",
			DurationMs: dur,
		}
	}

	findings := parseTruthsayerOutput(output)
	if ciMode && !cmdPass && findings.Errors == 0 && findings.Warnings == 0 && findings.Info == 0 {
		return verdict.GateResult{
			Name:       "truthsayer",
			Pass:       true,
			Skipped:    true,
			Output:     "truthsayer ci returned no findings (skipped)",
			DurationMs: dur,
			Findings:   &findings,
		}
	}
	pass := cmdPass && findings.Errors == 0

	summary := fmt.Sprintf("%d errors, %d warnings, %d info", findings.Errors, findings.Warnings, findings.Info)
	if !pass {
		summary = fmt.Sprintf("errors=%d warnings=%d info=%d (cmd_pass=%v)", findings.Errors, findings.Warnings, findings.Info, cmdPass)
	}

	return verdict.GateResult{
		Name:       "truthsayer",
		Pass:       pass,
		Output:     summary,
		DurationMs: dur,
		Findings:   &findings,
	}
}

// parseTruthsayerOutput extracts finding counts from truthsayer JSON output.
// It uses json.Decoder to robustly locate the JSON object even when the
// output is prefixed by non-JSON log lines. Falls back to counting
// severity prefixes in plain-text output if no valid JSON is found.
func parseTruthsayerOutput(output string) verdict.Findings {
	var f verdict.Findings
	raw := strings.TrimSpace(output)
	if raw == "" {
		return f
	}

	if summary, ok := parseTruthsayerSummary(raw); ok {
		return summary
	}

	// Locate the start of the JSON object. Output may contain log lines
	// before the JSON blob (e.g. "INFO scanning...").
	if idx := strings.Index(raw, "{"); idx >= 0 {
		var report truthsayerReport
		dec := json.NewDecoder(strings.NewReader(raw[idx:]))
		if err := dec.Decode(&report); err == nil {
			// Prefer the summary counts when present.
			if report.Summary.Errors > 0 || report.Summary.Warnings > 0 || report.Summary.Info > 0 {
				return verdict.Findings{
					Errors:   report.Summary.Errors,
					Warnings: report.Summary.Warnings,
					Info:     report.Summary.Info,
				}
			}
			// Summary might be all zeros; cross-check against findings array.
			if len(report.Findings) > 0 {
				for _, fd := range report.Findings {
					switch strings.ToLower(fd.Severity) {
					case "error":
						f.Errors++
					case "warning", "warn":
						f.Warnings++
					case "info":
						f.Info++
					}
				}
				return f
			}
			// Valid JSON with zero summary and no findings — clean scan.
			return verdict.Findings{
				Errors:   report.Summary.Errors,
				Warnings: report.Summary.Warnings,
				Info:     report.Summary.Info,
			}
		}
	}

	// Fallback: count severity prefixes in plain-text output.
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ERROR") {
			f.Errors++
		} else if strings.HasPrefix(trimmed, "WARN") {
			f.Warnings++
		} else if strings.HasPrefix(trimmed, "INFO") {
			f.Info++
		}
	}
	return f
}

var truthsayerSummaryRE = regexp.MustCompile(`Summary:\s+(\d+)\s+errors,\s+(\d+)\s+warnings,\s+(\d+)\s+info`)

func parseTruthsayerSummary(output string) (verdict.Findings, bool) {
	match := truthsayerSummaryRE.FindStringSubmatch(output)
	if len(match) != 4 {
		return verdict.Findings{}, false
	}
	var f verdict.Findings
	_, err := fmt.Sscanf(match[0], "Summary: %d errors, %d warnings, %d info", &f.Errors, &f.Warnings, &f.Info)
	return f, err == nil
}
