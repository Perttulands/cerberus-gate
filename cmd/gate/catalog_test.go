package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempRegistry(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}

func TestCatalogCheck_MixedStatuses(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "good-source.md")
	if err := os.WriteFile(sourceFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	reg := writeTempRegistry(t, tmp, `entries:
  - name: good-entry
    bins: [ls]
    source: `+sourceFile+`
    verify_commands:
      - "ls --help"
  - name: broken-entry
    bins: [nonexistent-binary-xyz]
  - name: stale-entry
    bins: [ls]
    source: /tmp/nonexistent-file-catalog-test
`)

	output := captureStdout(t, func() {
		code := runCatalogCheck(context.Background(), []string{"--registry", reg})
		if code != 1 {
			t.Errorf("expected exit 1 (has failures), got %d", code)
		}
	})

	if !strings.Contains(output, "good-entry") {
		t.Errorf("expected good-entry in output, got: %s", output)
	}
	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS in output, got: %s", output)
	}
	if !strings.Contains(output, "BROKEN") {
		t.Errorf("expected BROKEN in output, got: %s", output)
	}
	if !strings.Contains(output, "STALE") {
		t.Errorf("expected STALE in output, got: %s", output)
	}
}

func TestCatalogCheck_JSONOutput(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "source.md")
	if err := os.WriteFile(sourceFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	reg := writeTempRegistry(t, tmp, `entries:
  - name: good-entry
    bins: [ls]
    source: `+sourceFile+`
  - name: broken-entry
    bins: [nonexistent-binary-xyz]
  - name: stale-entry
    bins: [ls]
    source: /tmp/nonexistent-file-catalog-test
`)

	output := captureStdout(t, func() {
		code := runCatalogCheck(context.Background(), []string{"--registry", reg, "--json"})
		if code != 1 {
			t.Errorf("expected exit 1, got %d", code)
		}
	})

	var results []catalogResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &results); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, output)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	byName := make(map[string]catalogResult)
	for _, r := range results {
		byName[r.Name] = r
	}

	if byName["good-entry"].Status != "PASS" {
		t.Errorf("good-entry: expected PASS, got %q", byName["good-entry"].Status)
	}
	if byName["broken-entry"].Status != "BROKEN" {
		t.Errorf("broken-entry: expected BROKEN, got %q", byName["broken-entry"].Status)
	}
	if byName["stale-entry"].Status != "STALE" {
		t.Errorf("stale-entry: expected STALE, got %q", byName["stale-entry"].Status)
	}

	// BROKEN entry should have details about missing binary
	if len(byName["broken-entry"].Details) == 0 {
		t.Error("broken-entry: expected details about missing binary")
	}
	// STALE entry should have details about missing source
	if len(byName["stale-entry"].Details) == 0 {
		t.Error("stale-entry: expected details about missing source")
	}
}

func TestCatalogCheck_AllPass_ExitZero(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "source.md")
	if err := os.WriteFile(sourceFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	reg := writeTempRegistry(t, tmp, `entries:
  - name: only-good
    bins: [ls]
    source: `+sourceFile+`
`)

	captureStdout(t, func() {
		code := runCatalogCheck(context.Background(), []string{"--registry", reg, "--json"})
		if code != 0 {
			t.Errorf("expected exit 0 (all pass), got %d", code)
		}
	})
}

func TestCatalogCheck_FlagErrors(t *testing.T) {
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	tests := []struct {
		name string
		args []string
	}{
		{"--registry without value", []string{"--registry"}},
		{"unknown flag", []string{"--bogus"}},
		{"missing registry file", []string{"--registry", "/tmp/nonexistent-registry-xyz.yaml"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := runCatalogCheck(context.Background(), tt.args)
			if code != 1 {
				t.Fatalf("runCatalogCheck(%v) = %d, want 1", tt.args, code)
			}
		})
	}
}

func TestCatalogCheck_VerifyCommandFailure(t *testing.T) {
	tmp := t.TempDir()
	reg := writeTempRegistry(t, tmp, `entries:
  - name: cmd-fails
    bins: [ls]
    verify_commands:
      - "false"
`)

	output := captureStdout(t, func() {
		code := runCatalogCheck(context.Background(), []string{"--registry", reg, "--json"})
		if code != 1 {
			t.Errorf("expected exit 1, got %d", code)
		}
	})

	var results []catalogResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &results); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if results[0].Status != "BROKEN" {
		t.Errorf("expected BROKEN from failed verify command, got %q", results[0].Status)
	}
}
