package review

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVerdict_Valid(t *testing.T) {
	input := `{
		"bead_id": "pol-a05d",
		"verdict": "PARTIAL",
		"scope_items": [
			{"item": "All test-noise beads are closed", "status": "FAIL", "evidence": "6 open beads remain", "suggestion": "Close them"},
			{"item": "Probe scripts auto-close", "status": "PASS", "evidence": "trap-based cleanup found"}
		],
		"gate_check_consistent": true,
		"summary": "1 of 2 scope items unmet."
	}`

	v, err := ParseVerdict([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.BeadID != "pol-a05d" {
		t.Errorf("bead_id = %q, want pol-a05d", v.BeadID)
	}
	if v.Verdict != "PARTIAL" {
		t.Errorf("verdict = %q, want PARTIAL", v.Verdict)
	}
	if len(v.ScopeItems) != 2 {
		t.Errorf("scope_items count = %d, want 2", len(v.ScopeItems))
	}
	if v.ScopeItems[0].Status != "FAIL" {
		t.Errorf("scope_items[0].status = %q, want FAIL", v.ScopeItems[0].Status)
	}
	if v.ScopeItems[1].Suggestion != "" {
		t.Errorf("scope_items[1].suggestion should be empty, got %q", v.ScopeItems[1].Suggestion)
	}
}

func TestParseVerdict_InvalidVerdict(t *testing.T) {
	input := `{"bead_id":"x","verdict":"MAYBE","scope_items":[]}`
	_, err := ParseVerdict([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid verdict value")
	}
	if !strings.Contains(err.Error(), "MAYBE") {
		t.Errorf("error should mention invalid value: %v", err)
	}
}

func TestParseVerdict_InvalidJSON(t *testing.T) {
	_, err := ParseVerdict([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseVerdict_MissingScopeItemField(t *testing.T) {
	input := `{"bead_id":"x","verdict":"PASS","scope_items":[{"item":"","status":"PASS"}]}`
	_, err := ParseVerdict([]byte(input))
	if err == nil {
		t.Fatal("expected error for empty item field")
	}
}

func TestParseVerdict_InvalidScopeStatus(t *testing.T) {
	input := `{"bead_id":"x","verdict":"PASS","scope_items":[{"item":"test","status":"DONE"}]}`
	_, err := ParseVerdict([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid scope item status")
	}
}

func TestExitCodeForVerdict(t *testing.T) {
	tests := []struct {
		verdict string
		want    int
	}{
		{"PASS", 0},
		{"PARTIAL", 1},
		{"FAIL", 1},
		{"UNCLEAR", 2},
	}
	for _, tt := range tests {
		v := &ReviewVerdict{Verdict: tt.verdict}
		got := ExitCodeForVerdict(v)
		if got != tt.want {
			t.Errorf("ExitCodeForVerdict(%s) = %d, want %d", tt.verdict, got, tt.want)
		}
	}
}

func TestFormatPrettyVerdict_ContainsKey(t *testing.T) {
	v := &ReviewVerdict{
		BeadID:  "pol-test",
		Verdict: "PARTIAL",
		ScopeItems: []ScopeItem{
			{Item: "tests pass", Status: "PASS", Evidence: "go test ok"},
			{Item: "docs updated", Status: "FAIL", Evidence: "README unchanged", Suggestion: "Update README"},
		},
		Summary: "1 of 2 scope items met.",
	}
	out := FormatPrettyVerdict(v)
	if !strings.Contains(out, "REVIEW VERDICT: PARTIAL") {
		t.Error("missing verdict header")
	}
	if !strings.Contains(out, "tests pass") {
		t.Error("missing scope item text")
	}
	if !strings.Contains(out, "Update README") {
		t.Error("missing suggestion")
	}
	if !strings.Contains(out, "1 of 2 scope items met") {
		t.Error("missing score line")
	}
}

func TestReviewVerdictJSON_Roundtrip(t *testing.T) {
	v := &ReviewVerdict{
		BeadID:              "pol-a05d",
		Verdict:             "PASS",
		ScopeItems:          []ScopeItem{{Item: "builds", Status: "PASS", Evidence: "ok"}},
		GateCheckConsistent: true,
		Summary:             "all good",
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	v2, err := ParseVerdict(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v2.BeadID != v.BeadID || v2.Verdict != v.Verdict || len(v2.ScopeItems) != 1 {
		t.Error("roundtrip mismatch")
	}
}
