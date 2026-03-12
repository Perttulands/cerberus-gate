package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidLevel(t *testing.T) {
	tests := []struct {
		level string
		want  bool
	}{
		{"quick", true},
		{"standard", true},
		{"deep", true},
		{"", false},
		{"ultra", false},
	}
	for _, tt := range tests {
		if got := ValidLevel(tt.level); got != tt.want {
			t.Errorf("ValidLevel(%q) = %v, want %v", tt.level, got, tt.want)
		}
	}
}

func TestRun_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	v := Run(context.Background(), dir, LevelQuick, "tester")

	if v.Citizen != "tester" {
		t.Errorf("expected citizen 'tester', got %q", v.Citizen)
	}
	if v.Level != LevelQuick {
		t.Errorf("expected level 'quick', got %q", v.Level)
	}
	// Empty dir — no tests, no linters — should pass
	if !v.Pass {
		t.Errorf("expected pass for empty dir, got fail: %+v", v.Gates)
	}
}

func TestRun_QuickLevel_OnlyTestsAndLint(t *testing.T) {
	dir := t.TempDir()
	v := Run(context.Background(), dir, LevelQuick, "tester")

	for _, g := range v.Gates {
		if g.Name == "truthsayer" || g.Name == "ubs" || g.Name == "risk" {
			t.Errorf("quick level should not include %s gate", g.Name)
		}
	}
}

func TestRun_StandardLevel_IncludesTruthsayerAndUBS(t *testing.T) {
	dir := t.TempDir()
	v := Run(context.Background(), dir, LevelStandard, "tester")

	hasTS, hasUBS := false, false
	for _, g := range v.Gates {
		if g.Name == "truthsayer" {
			hasTS = true
		}
		if g.Name == "ubs" {
			hasUBS = true
		}
	}
	if !hasTS {
		t.Error("standard level should include truthsayer")
	}
	if !hasUBS {
		t.Error("standard level should include ubs")
	}
}

func TestRun_DeepLevel_DoesNotIncludePlaceholderRisk(t *testing.T) {
	dir := t.TempDir()
	v := Run(context.Background(), dir, LevelDeep, "tester")

	for _, g := range v.Gates {
		if g.Name == "risk" {
			t.Fatal("deep level should not include placeholder risk gate")
		}
	}
}

func TestRun_GoProject_RunsGoTest(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal Go project that passes tests
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testproject\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)

	v := Run(context.Background(), dir, LevelQuick, "tester")

	hasTests := false
	for _, g := range v.Gates {
		if g.Name == "tests" {
			hasTests = true
			if !g.Pass {
				t.Errorf("go test should pass on trivial project, output: %s", g.Output)
			}
		}
	}
	if !hasTests {
		t.Error("expected tests gate in results")
	}
}
