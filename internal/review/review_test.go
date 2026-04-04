package review

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"polis/gate/internal/verdict"
)

// TestIntegration_ContextAssembly verifies that context assembly works with
// a real git repo, testing FetchDiff, FetchBranch, and FormatBundle.
func TestIntegration_ContextAssembly(t *testing.T) {
	repo := setupTestRepo(t)

	// Test FetchDiff
	diff, err := FetchDiff(repo)
	if err != nil {
		t.Fatalf("FetchDiff: %v", err)
	}
	if diff == "" {
		t.Error("diff should not be empty — repo has staged changes")
	}
	if !strings.Contains(diff, "cleanup.sh") {
		t.Error("diff should reference cleanup.sh (the incomplete fix)")
	}

	// Test FetchBranch
	branch := FetchBranch(repo)
	if branch == "" || branch == "unknown" {
		t.Errorf("branch = %q, want a valid branch name", branch)
	}

	// Build context bundle manually (no br dependency)
	bundle := &ContextBundle{
		Bead: &BeadInfo{
			ID:          "pol-test-a05d",
			Title:       "Close test-noise beads and fix probe cleanup",
			Description: "All test-noise beads are closed, probe scripts auto-close. Done when: all test-noise beads closed, probes auto-cleanup, br list clean.",
			Status:      "in_progress",
			Priority:    0,
			Labels:      []string{"test-noise", "cleanup"},
		},
		Diff:   diff,
		Repo:   repo,
		Branch: branch,
		FastPath: &verdict.Verdict{
			Pass:  true,
			Score: 1.0,
			Gates: []verdict.GateResult{{Name: "tests", Pass: true}},
		},
	}

	// Verify FormatBundle includes all sections
	out := FormatBundle(bundle)
	for _, want := range []string{"BEAD:", "REPO:", "FAST-PATH:", "DIFF:", "pol-test-a05d"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatBundle missing %q", want)
		}
	}
}

// TestIntegration_PromptWriteAndRead verifies the full prompt file lifecycle.
func TestIntegration_PromptWriteAndRead(t *testing.T) {
	bundle := &ContextBundle{
		Bead: &BeadInfo{
			ID:          "pol-prompt-test",
			Title:       "Fix probe cleanup",
			Description: "Done when: all probes auto-close test beads.",
		},
		Diff:   "diff --git a/cleanup.sh b/cleanup.sh\n+trap cleanup EXIT",
		Repo:   "/tmp/test-repo",
		Branch: "main",
		FastPath: &verdict.Verdict{
			Pass:  true,
			Score: 1.0,
			Gates: []verdict.GateResult{{Name: "tests", Pass: true}},
		},
	}

	path, err := WritePromptFile(bundle)
	if err != nil {
		t.Fatalf("WritePromptFile: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	content := string(data)

	checks := []string{
		"You are Cerberus",
		"pol-prompt-test",
		"all probes auto-close test beads",
		"cleanup.sh",
		"verdict.json",
		"scope_items",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// TestIntegration_VerdictParsing_PolA05D simulates the pol-a05d failure pattern.
// A PARTIAL verdict identifies that test-noise beads from Go tests remain open.
func TestIntegration_VerdictParsing_PolA05D(t *testing.T) {
	mockVerdict := ReviewVerdict{
		BeadID:  "pol-a05d",
		Verdict: VerdictPartial,
		ScopeItems: []ScopeItem{
			{
				Item:     "Probe scripts auto-close their test beads",
				Status:   "PASS",
				Evidence: "smoke.sh, probe_br_bv.sh have trap-based cleanup",
			},
			{
				Item:       "All test-noise beads are closed",
				Status:     "FAIL",
				Evidence:   "br list shows 6 open beads with test-noise titles (pol-de16, pol-ec42, pol-2e2b, pol-4316, pol-4b80, pol-e4ae)",
				Suggestion: "These come from Go test files, not bash probes",
			},
			{
				Item:     "br list shows only real work items after cleanup",
				Status:   "FAIL",
				Evidence: "br list still contains 6 test-noise beads from Go test files",
			},
		},
		GateCheckConsistent: true,
		Summary:             "Diff addresses bash probe scripts but misses Go test files. 2 of 3 scope items unmet.",
	}

	data, err := json.Marshal(mockVerdict)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := ParseVerdict(data)
	if err != nil {
		t.Fatalf("ParseVerdict: %v", err)
	}

	if parsed.Verdict != VerdictPartial {
		t.Errorf("verdict = %q, want PARTIAL", parsed.Verdict)
	}
	if len(parsed.ScopeItems) != 3 {
		t.Errorf("scope items = %d, want 3", len(parsed.ScopeItems))
	}

	// Exit code 1 for PARTIAL
	if ExitCodeForVerdict(parsed) != 1 {
		t.Errorf("exit code = %d, want 1", ExitCodeForVerdict(parsed))
	}

	// Pretty output catches the gap
	pretty := FormatPrettyVerdict(parsed)
	if !strings.Contains(pretty, "PARTIAL") {
		t.Error("pretty output should show PARTIAL")
	}
	if !strings.Contains(pretty, "6 open beads") {
		t.Error("pretty output should show evidence about remaining beads")
	}
	if !strings.Contains(pretty, "1 of 3 scope items met") {
		t.Error("pretty output should show 1 of 3 met")
	}
}

// TestIntegration_VerdictFile_Lifecycle simulates agent writing verdict file,
// gate reading and parsing it.
func TestIntegration_VerdictFile_Lifecycle(t *testing.T) {
	beadID := "pol-e2e-lifecycle"
	verdictPath := VerdictPath(beadID)

	agentVerdict := ReviewVerdict{
		BeadID:  beadID,
		Verdict: VerdictPass,
		ScopeItems: []ScopeItem{
			{Item: "tests pass", Status: "PASS", Evidence: "go test ok"},
			{Item: "lint clean", Status: "PASS", Evidence: "go vet ok"},
		},
		GateCheckConsistent: true,
		Summary:             "All scope items met.",
	}
	data, err := json.MarshalIndent(agentVerdict, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(verdictPath, data, 0o644); err != nil {
		t.Fatalf("write verdict: %v", err)
	}
	defer os.Remove(verdictPath)

	readData, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	parsed, err := ParseVerdict(readData)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Verdict != VerdictPass {
		t.Errorf("verdict = %q, want PASS", parsed.Verdict)
	}
	if ExitCodeForVerdict(parsed) != 0 {
		t.Error("PASS verdict should produce exit code 0")
	}
}

// TestIntegration_DoneConditionExtraction verifies extraction of done-conditions.
func TestIntegration_DoneConditionExtraction(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantSubstr  string
	}{
		{
			"done when marker",
			"Some context. Done when: all beads closed and probes clean.",
			"all beads closed and probes clean.",
		},
		{
			"DONE WHEN uppercase",
			"Task desc. DONE WHEN: tests pass with 100% coverage.",
			"tests pass with 100% coverage.",
		},
		{
			"no marker falls back",
			"Fix the bug in the parser and add tests.",
			"Fix the bug in the parser",
		},
		{
			"done-condition marker",
			"Implement X. Done-condition: X works and Y is tested.",
			"X works and Y is tested.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bead := &BeadInfo{Title: "test", Description: tt.description}
			got := extractDoneCondition(bead)
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("got %q, want substring %q", got, tt.wantSubstr)
			}
		})
	}
}

// TestIntegration_FormatBundle verifies the human-readable output.
func TestIntegration_FormatBundle(t *testing.T) {
	bundle := &ContextBundle{
		Bead: &BeadInfo{
			ID: "pol-fmt", Title: "Test", Description: "Done when: ok.",
			Status: "open", Priority: 0, Labels: []string{"test", "gate"},
		},
		Diff: "diff --git a/main.go b/main.go\n+new line", Repo: "/tmp/test", Branch: "feature",
		FastPath: &verdict.Verdict{
			Pass: true, Score: 1.0,
			Gates: []verdict.GateResult{
				{Name: "tests", Pass: true, DurationMs: 100},
				{Name: "lint", Pass: true, DurationMs: 50},
			},
		},
	}

	out := FormatBundle(bundle)
	for _, want := range []string{
		"BEAD:", "id: pol-fmt", "REPO:", "FAST-PATH:", "gate-score: 1.00 (PASS)", "DIFF:", "new line",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// TestIntegration_PromptPath verifies path format.
func TestIntegration_PromptPath(t *testing.T) {
	path := PromptPath("pol-abc123")
	if !strings.Contains(path, "gate-review-pol-abc123.md") {
		t.Errorf("PromptPath = %q, want gate-review-pol-abc123.md", path)
	}
}

// TestIntegration_VerdictPath verifies verdict path format.
func TestIntegration_VerdictPath(t *testing.T) {
	path := VerdictPath("pol-abc123")
	if !strings.Contains(path, "gate-review-pol-abc123-verdict.json") {
		t.Errorf("VerdictPath = %q, want gate-review-pol-abc123-verdict.json", path)
	}
}

// --- Test helpers ---

// setupTestRepo creates a temporary git repo with a deliberately incomplete fix.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s: %s", args, err, out)
		}
	}

	// Initial commit: bash probe without cleanup + Go test that leaks beads
	writeTestFile(t, dir, "probes/smoke.sh", "#!/bin/bash\nbr create test-noise -t chore\necho done\n")
	writeTestFile(t, dir, "tests/bead_test.go", "package tests\nimport \"testing\"\nfunc TestBead(t *testing.T) { t.Log(\"creating beads...\") }\n")

	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial")

	// Incomplete fix: only bash probes patched, Go test left unfixed
	writeTestFile(t, dir, "probes/cleanup.sh", "#!/bin/bash\ntrap 'br close $BEAD_ID --reason cleanup' EXIT\nbr create test-noise -t chore\n")

	gitRun(t, dir, "add", ".")
	return dir
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %s", args, err, out)
	}
}
