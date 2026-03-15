package bead

import (
	"errors"
	"strings"
	"testing"

	"polis/gate/internal/city"
	"polis/gate/internal/verdict"
)

func TestRecordCity_UsesBRWhenAvailable(t *testing.T) {
	defer resetHooksForTest()

	var createArgs []string
	lookPath = func(name string) (string, error) {
		if name == "br" {
			return "/usr/bin/br", nil
		}
		return "", errors.New("missing")
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		createArgs = append([]string{}, args...)
		return []byte("pol-123\n"), nil
	}

	id := RecordCity(city.Verdict{
		Repo:     "relay",
		Status:   "fail",
		ExitCode: city.ExitFail,
		Summary:  city.Summary{Fail: 1, Skip: 2},
		Checks: []city.CheckResult{
			{Name: "boundary", Status: city.StatusFail, Detail: "not ignored"},
		},
	}, "tester")

	if id != "pol-123" {
		t.Fatalf("expected bead id pol-123, got %q", id)
	}
	joined := strings.Join(createArgs, " ")
	if !strings.Contains(joined, "gate city: relay (fail)") {
		t.Fatalf("expected city title in args: %v", createArgs)
	}
	if !strings.Contains(joined, "kind:city") {
		t.Fatalf("expected city labels in args: %v", createArgs)
	}
}

func TestRecord_UsesBR(t *testing.T) {
	defer resetHooksForTest()

	lookPath = func(name string) (string, error) {
		if name == "br" {
			return "/usr/bin/br", nil
		}
		return "", errors.New("missing")
	}

	var createArgs []string
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		createArgs = append([]string{}, args...)
		return []byte("br-9\n"), nil
	}

	id := Record(verdict.Verdict{
		Pass:    false,
		Level:   "standard",
		Citizen: "tester",
		Repo:    "gate",
		Gates:   []verdict.GateResult{{Name: "tests", Pass: false}},
	})

	if id != "br-9" {
		t.Fatalf("expected br-9 id, got %q", id)
	}
	joined := strings.Join(createArgs, " ")
	if !strings.Contains(joined, "gate gate") {
		t.Fatalf("expected title in args: %v", createArgs)
	}
}

func TestRecord_NoBRReturnsEmpty(t *testing.T) {
	defer resetHooksForTest()

	lookPath = func(name string) (string, error) { return "", errors.New("missing") }
	runCmd = func(name string, args ...string) ([]byte, error) {
		t.Fatalf("runCmd should not be called when br unavailable")
		return nil, nil
	}

	id := Record(verdict.Verdict{
		Pass:  true,
		Level: "quick",
		Repo:  "gate",
	})
	if id != "" {
		t.Fatalf("expected empty id, got %q", id)
	}
}

func TestRecord_NoToolReturnsEmpty(t *testing.T) {
	defer resetHooksForTest()

	lookPath = func(name string) (string, error) { return "", errors.New("missing") }
	runCmd = func(name string, args ...string) ([]byte, error) {
		t.Fatalf("runCmd should not be called when tools unavailable")
		return nil, nil
	}

	id := RecordCity(city.Verdict{Repo: "x", Status: "fail"}, "tester")
	if id != "" {
		t.Fatalf("expected empty id, got %q", id)
	}
}

func TestFormatCheckDescription_IncludesGates(t *testing.T) {
	v := verdict.Verdict{
		Pass:  false,
		Level: "standard",
		Repo:  "test-repo",
		Gates: []verdict.GateResult{
			{Name: "tests", Pass: true, DurationMs: 100},
			{Name: "lint:go vet", Pass: false, DurationMs: 50},
			{Name: "truthsayer", Pass: true, Skipped: true, DurationMs: 0},
		},
	}

	out := formatCheckDescription(v)

	if !strings.Contains(out, "gate check verdict: fail") {
		t.Fatalf("expected verdict status, got: %q", out)
	}
	if !strings.Contains(out, "repo: test-repo") {
		t.Fatalf("expected repo, got: %q", out)
	}
	if !strings.Contains(out, "level: standard") {
		t.Fatalf("expected level, got: %q", out)
	}
	if !strings.Contains(out, "- tests: pass") {
		t.Fatalf("expected tests gate, got: %q", out)
	}
	if !strings.Contains(out, "- lint:go vet: fail") {
		t.Fatalf("expected lint failure, got: %q", out)
	}
	if !strings.Contains(out, "- truthsayer: skip") {
		t.Fatalf("expected truthsayer skip, got: %q", out)
	}
}

func TestFormatCheckDescription_PassVerdict(t *testing.T) {
	v := verdict.Verdict{
		Pass:  true,
		Level: "quick",
		Repo:  "gate",
		Gates: []verdict.GateResult{
			{Name: "tests", Pass: true, DurationMs: 200},
		},
	}

	out := formatCheckDescription(v)

	if !strings.Contains(out, "gate check verdict: pass") {
		t.Fatalf("expected pass verdict, got: %q", out)
	}
}

func TestNormalizeLabels(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"c,b,a", "a,b,c"},
		{"tool:gate,status:pass,repo:x", "repo:x,status:pass,tool:gate"},
		{"single", "single"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLabels(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeLabels(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRecord_FailureVerdictTitle(t *testing.T) {
	defer resetHooksForTest()

	var createArgs []string
	lookPath = func(name string) (string, error) {
		if name == "br" {
			return "/usr/bin/br", nil
		}
		return "", errors.New("missing")
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		createArgs = append([]string{}, args...)
		return []byte("pol-fail-1\n"), nil
	}

	id := Record(verdict.Verdict{
		Pass:    false,
		Level:   "deep",
		Citizen: "auditor",
		Repo:    "relay",
		Gates: []verdict.GateResult{
			{Name: "tests", Pass: true, DurationMs: 100},
			{Name: "lint:go vet", Pass: false, DurationMs: 50},
		},
	})

	if id != "pol-fail-1" {
		t.Fatalf("expected pol-fail-1, got %q", id)
	}

	joined := strings.Join(createArgs, " ")
	if !strings.Contains(joined, "relay gate deep: fail") {
		t.Fatalf("expected failure title, got: %s", joined)
	}
	if !strings.Contains(joined, "status:fail") {
		t.Fatalf("expected status:fail label, got: %s", joined)
	}
	if !strings.Contains(joined, "-a auditor") {
		t.Fatalf("expected assignee, got: %s", joined)
	}
}

func TestRecord_UnknownCitizenSkipsAssignee(t *testing.T) {
	defer resetHooksForTest()

	var createArgs []string
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		createArgs = append([]string{}, args...)
		return []byte("pol-1\n"), nil
	}

	Record(verdict.Verdict{
		Pass:    false,
		Level:   "quick",
		Citizen: "unknown",
		Repo:    "test",
	})

	joined := strings.Join(createArgs, " ")
	if strings.Contains(joined, "-a") {
		t.Fatalf("expected no assignee for unknown citizen, got: %s", joined)
	}
}

func TestCreateWithBR_CommandFails(t *testing.T) {
	defer resetHooksForTest()

	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		return nil, errors.New("br crashed")
	}

	id := Record(verdict.Verdict{Pass: false, Level: "quick", Repo: "x"})
	if id != "" {
		t.Fatalf("expected empty id on br failure, got %q", id)
	}
}

func TestFormatCityDescription_IncludesChecks(t *testing.T) {
	out := formatCityDescription(city.Verdict{
		Repo:     "relay",
		Status:   "fail",
		ExitCode: city.ExitFail,
		Summary:  city.Summary{Pass: 1, Fail: 2, Skip: 1},
		Checks: []city.CheckResult{
			{Name: "boundary", Status: city.StatusPass, Detail: "ok", DurationMs: 3},
			{Name: "split", Status: city.StatusFail, Detail: "missing", DurationMs: 2},
		},
	})
	if !strings.Contains(out, "gate city verdict: fail") {
		t.Fatalf("missing status in description: %q", out)
	}
	if !strings.Contains(out, "- split: fail") {
		t.Fatalf("missing check line in description: %q", out)
	}
}

// --- New lifecycle tests ---

func TestRecord_PassCreatesNoBead(t *testing.T) {
	defer resetHooksForTest()

	var createCalled bool
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		createCalled = true
		return []byte("should-not-happen\n"), nil
	}

	id := Record(verdict.Verdict{
		Pass:  true,
		Level: "standard",
		Repo:  "relay",
		Gates: []verdict.GateResult{{Name: "tests", Pass: true}},
	})

	if id != "" {
		t.Fatalf("expected empty id for pass verdict, got %q", id)
	}
	if createCalled {
		t.Fatalf("br create should not be called for pass verdict")
	}
}

func TestRecord_PassClosesOpenFailBead(t *testing.T) {
	defer resetHooksForTest()

	var closedID string
	var closeReason string
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte(`[{"id":"pol-old-fail","labels":["tool:gate","repo:relay","status:fail","level:standard"]}]`), nil
		}
		if len(args) > 0 && args[0] == "close" {
			closedID = args[1]
			for i, a := range args {
				if a == "--reason" && i+1 < len(args) {
					closeReason = args[i+1]
				}
			}
			return []byte(""), nil
		}
		t.Fatalf("unexpected br subcommand: %v", args)
		return nil, nil
	}

	id := Record(verdict.Verdict{
		Pass:  true,
		Level: "standard",
		Repo:  "relay",
		Gates: []verdict.GateResult{{Name: "tests", Pass: true}},
	})

	if id != "" {
		t.Fatalf("expected empty id for pass verdict, got %q", id)
	}
	if closedID != "pol-old-fail" {
		t.Fatalf("expected close of pol-old-fail, got %q", closedID)
	}
	if !strings.Contains(closeReason, "Gate now passing") {
		t.Fatalf("expected close reason with 'Gate now passing', got %q", closeReason)
	}
}

func TestRecord_FailDeduplicatesExistingBead(t *testing.T) {
	defer resetHooksForTest()

	var createCalled bool
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte(`[{"id":"pol-existing","labels":["tool:gate","repo:relay","status:fail","level:standard"]}]`), nil
		}
		createCalled = true
		return []byte("should-not-happen\n"), nil
	}

	id := Record(verdict.Verdict{
		Pass:  false,
		Level: "standard",
		Repo:  "relay",
		Gates: []verdict.GateResult{{Name: "tests", Pass: false}},
	})

	if id != "pol-existing" {
		t.Fatalf("expected existing bead id pol-existing, got %q", id)
	}
	if createCalled {
		t.Fatalf("br create should not be called when existing fail bead found")
	}
}

func TestRecord_FailCreatesNewWhenNoneExists(t *testing.T) {
	defer resetHooksForTest()

	var createArgs []string
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		createArgs = append([]string{}, args...)
		return []byte("pol-new\n"), nil
	}

	id := Record(verdict.Verdict{
		Pass:    false,
		Level:   "deep",
		Citizen: "tester",
		Repo:    "relay",
		Gates:   []verdict.GateResult{{Name: "lint", Pass: false}},
	})

	if id != "pol-new" {
		t.Fatalf("expected pol-new, got %q", id)
	}
	if len(createArgs) == 0 {
		t.Fatalf("expected create to be called")
	}
	joined := strings.Join(createArgs, " ")
	if !strings.Contains(joined, "relay gate deep: fail") {
		t.Fatalf("expected title in create args, got: %s", joined)
	}
}

func TestRecordCity_NonFailCreatesNoBead(t *testing.T) {
	defer resetHooksForTest()

	var createCalled bool
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte("[]"), nil
		}
		createCalled = true
		return []byte("should-not-happen\n"), nil
	}

	id := RecordCity(city.Verdict{
		Repo:   "relay",
		Status: "warn",
	}, "tester")

	if id != "" {
		t.Fatalf("expected empty id for non-fail city verdict, got %q", id)
	}
	if createCalled {
		t.Fatalf("br create should not be called for non-fail city verdict")
	}
}

func TestRecordCity_FailDeduplicates(t *testing.T) {
	defer resetHooksForTest()

	var createCalled bool
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte(`[{"id":"pol-city-dup","labels":["tool:gate","repo:relay","status:fail","kind:city"]}]`), nil
		}
		createCalled = true
		return []byte("should-not-happen\n"), nil
	}

	id := RecordCity(city.Verdict{
		Repo:   "relay",
		Status: "fail",
	}, "tester")

	if id != "pol-city-dup" {
		t.Fatalf("expected existing city bead id, got %q", id)
	}
	if createCalled {
		t.Fatalf("br create should not be called when existing city fail bead found")
	}
}

func TestRecordCity_PassClosesOpenFailBead(t *testing.T) {
	defer resetHooksForTest()

	var closedID string
	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}
	runCmd = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "search" {
			return []byte(`[{"id":"pol-city-fail","labels":["tool:gate","repo:relay","status:fail","kind:city"]}]`), nil
		}
		if len(args) > 0 && args[0] == "close" {
			closedID = args[1]
			return []byte(""), nil
		}
		t.Fatalf("unexpected br subcommand: %v", args)
		return nil, nil
	}

	id := RecordCity(city.Verdict{
		Repo:   "relay",
		Status: "pass",
	}, "tester")

	if id != "" {
		t.Fatalf("expected empty id for pass city verdict, got %q", id)
	}
	if closedID != "pol-city-fail" {
		t.Fatalf("expected close of pol-city-fail, got %q", closedID)
	}
}

func TestParseFirstBeadID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty array", "[]", ""},
		{"single result", `[{"id":"pol-1"}]`, "pol-1"},
		{"multiple results", `[{"id":"pol-1"},{"id":"pol-2"}]`, "pol-1"},
		{"invalid json", "not-json", ""},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFirstBeadID(tt.input)
			if got != tt.want {
				t.Fatalf("parseFirstBeadID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindOpenFailBead_SearchLabels(t *testing.T) {
	defer resetHooksForTest()

	lookPath = func(name string) (string, error) {
		return "/usr/bin/br", nil
	}

	// v2: search does not use --label flags; labels are filtered client-side.
	// Verify that search args use --status open (no --label).
	var searchArgs []string
	runCmd = func(name string, args ...string) ([]byte, error) {
		searchArgs = append([]string{}, args...)
		return []byte("[]"), nil
	}

	findOpenFailBead("relay", "standard")
	joined := strings.Join(searchArgs, " ")
	if strings.Contains(joined, "--label") {
		t.Fatalf("v2 search should not use --label, got: %s", joined)
	}
	if !strings.Contains(joined, "--status open") {
		t.Fatalf("expected --status open in search, got: %s", joined)
	}

	// With level: should match bead with matching labels
	runCmd = func(name string, args ...string) ([]byte, error) {
		return []byte(`[{"id":"pol-match","labels":["tool:gate","repo:relay","status:fail","level:standard"]},{"id":"pol-nomatch","labels":["tool:gate","repo:other","status:fail"]}]`), nil
	}
	id := findOpenFailBead("relay", "standard")
	if id != "pol-match" {
		t.Fatalf("expected pol-match, got %q", id)
	}

	// Without level: should match kind:city labels
	runCmd = func(name string, args ...string) ([]byte, error) {
		return []byte(`[{"id":"pol-city","labels":["tool:gate","repo:relay","status:fail","kind:city"]}]`), nil
	}
	id = findOpenFailBead("relay", "")
	if id != "pol-city" {
		t.Fatalf("expected pol-city, got %q", id)
	}
}
