package bead

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/Perttulands/polis-utils/brclient"

	"polis/gate/internal/city"
	"polis/gate/internal/verdict"
)

var (
	sharedBRClient = brclient.New()
	lookPath       = func(name string) (string, error) {
		if name == "br" {
			if sharedBRClient.Available() {
				return "br", nil
			}
			return "", exec.ErrNotFound
		}
		return exec.LookPath(name)
	}
	runCmd   = func(name string, args ...string) ([]byte, error) {
		if name == "br" {
			result, err := sharedBRClient.Run(context.Background(), brclient.Invocation{Args: args})
			if err != nil {
				return append(result.Stdout, result.Stderr...), err
			}
			return result.Stdout, nil
		}
		return exec.Command(name, args...).Output()
	}
)

// Record creates a bead for a gate check verdict.
// Fail-only: pass verdicts create no bead (and auto-resolve any open fail bead).
// Dedup: fail verdicts reuse an existing open fail bead if one exists.
func Record(v verdict.Verdict) string {
	if _, err := lookPath("br"); err != nil {
		return ""
	}

	status := "pass"
	if !v.Pass {
		status = "fail"
	}
	title := fmt.Sprintf("%s gate %s: %s", v.Repo, v.Level, status)

	// Pass: resolve any open fail bead, create nothing.
	if v.Pass {
		resolveOpenFailBead(v.Repo, v.Level, title)
		return ""
	}

	// Fail: deduplicate.
	if existing := findOpenFailBead(v.Repo, v.Level); existing != "" {
		return existing
	}

	labels := fmt.Sprintf("tool:gate,status:%s,repo:%s,level:%s", status, v.Repo, v.Level)
	description := formatCheckDescription(v)
	return createWithBR(title, labels, description, v.Citizen)
}

// RecordCity creates a bead for a gate city verdict.
// Fail-only: non-fail verdicts create no bead (and auto-resolve any open fail bead).
// Dedup: fail verdicts reuse an existing open fail bead if one exists.
func RecordCity(v city.Verdict, citizen string) string {
	if _, err := lookPath("br"); err != nil {
		return ""
	}

	title := fmt.Sprintf("gate city: %s (%s)", v.Repo, v.Status)

	// Non-fail (pass/warn): resolve any open fail bead, create nothing.
	if v.Status != "fail" {
		resolveOpenFailBead(v.Repo, "", title)
		return ""
	}

	// Fail: deduplicate.
	if existing := findOpenFailBead(v.Repo, ""); existing != "" {
		return existing
	}

	labels := fmt.Sprintf("tool:gate,kind:city,status:%s,repo:%s", v.Status, v.Repo)
	description := formatCityDescription(v)
	return createWithBR(title, labels, description, citizen)
}

// findOpenFailBead searches for an existing open fail bead for the given repo.
// For check verdicts pass the level; for city verdicts pass "" (searches kind:city instead).
func findOpenFailBead(repo, level string) string {
	// v2 search does not support --label; use text search + status filter
	// then match labels client-side.
	args := []string{
		"search", "gate",
		"--status", "open",
		"--json",
	}
	out, err := runCmd("br", args...)
	if err != nil {
		return ""
	}

	required := []string{"tool:gate", "repo:" + repo, "status:fail"}
	if level != "" {
		required = append(required, "level:"+level)
	} else {
		required = append(required, "kind:city")
	}
	return parseFirstBeadIDWithLabels(string(out), required)
}

// resolveOpenFailBead finds and closes any open fail bead for the given repo.
func resolveOpenFailBead(repo, level, summary string) {
	id := findOpenFailBead(repo, level)
	if id == "" {
		return
	}
	reason := fmt.Sprintf("Gate now passing: %s", summary)
	runCmd("br", "close", id, "--reason", reason)
}

type brSearchResult struct {
	ID     string   `json:"id"`
	Labels []string `json:"labels"`
}

func parseFirstBeadID(jsonOutput string) string {
	var results []brSearchResult
	if err := json.Unmarshal([]byte(jsonOutput), &results); err != nil {
		return ""
	}
	if len(results) == 0 {
		return ""
	}
	return results[0].ID
}

// parseFirstBeadIDWithLabels returns the first bead ID that has all required labels.
func parseFirstBeadIDWithLabels(jsonOutput string, requiredLabels []string) string {
	var results []brSearchResult
	if err := json.Unmarshal([]byte(jsonOutput), &results); err != nil {
		return ""
	}
	for _, r := range results {
		if hasAllLabels(r.Labels, requiredLabels) {
			return r.ID
		}
	}
	return ""
}

func hasAllLabels(labels, required []string) bool {
	set := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		set[l] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}

func createWithBR(title, labels, description, citizen string) string {
	if _, err := lookPath("br"); err != nil {
		return ""
	}
	args := []string{
		"create",
		title,
		"-t", "chore",
		"--labels", labels,
		"--description", description,
		"--silent",
	}
	if citizen != "" && citizen != "unknown" {
		args = append(args, "-a", citizen)
	}
	out, err := runCmd("br", args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func formatCheckDescription(v verdict.Verdict) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("gate check verdict: %s", boolStatus(v.Pass)))
	lines = append(lines, fmt.Sprintf("repo: %s", v.Repo))
	lines = append(lines, fmt.Sprintf("level: %s", v.Level))
	lines = append(lines, "checks:")
	for _, g := range v.Gates {
		status := boolStatus(g.Pass)
		if g.Skipped {
			status = "skip"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s (%dms)", g.Name, status, g.DurationMs))
	}
	return strings.Join(lines, "\n")
}

func formatCityDescription(v city.Verdict) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("gate city verdict: %s", v.Status))
	lines = append(lines, fmt.Sprintf("repo: %s", v.Repo))
	lines = append(lines, fmt.Sprintf("exit_code: %d", v.ExitCode))
	lines = append(lines, fmt.Sprintf("summary: pass=%d fail=%d skip=%d", v.Summary.Pass, v.Summary.Fail, v.Summary.Skip))
	lines = append(lines, "")
	lines = append(lines, "checks:")
	for _, c := range v.Checks {
		lines = append(lines, fmt.Sprintf("- %s: %s (%dms) %s", c.Name, c.Status, c.DurationMs, c.Detail))
	}
	return strings.Join(lines, "\n")
}

func boolStatus(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}

// resetHooksForTest restores package globals changed in tests.
func resetHooksForTest() {
	lookPath = exec.LookPath
	runCmd = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).Output()
	}
}

// normalizeLabels returns labels sorted lexicographically to simplify assertions.
func normalizeLabels(v string) string {
	parts := strings.Split(v, ",")
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
