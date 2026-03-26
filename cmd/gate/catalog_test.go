package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTempRegistry(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}

func TestCatalogCheck_MissingRegistryFile(t *testing.T) {
	code := runCatalogCheck(context.Background(), []string{"--registry", "/tmp/nonexistent-registry-xyz.yaml"})
	if code != 1 {
		t.Fatalf("expected exit 1 for missing registry, got %d", code)
	}
}

func TestCatalogCheck_MalformedYAML(t *testing.T) {
	tmp := t.TempDir()
	reg := writeTempRegistry(t, tmp, `entries: [[[invalid yaml`)
	code := runCatalogCheck(context.Background(), []string{"--registry", reg})
	if code != 1 {
		t.Fatalf("expected exit 1 for malformed YAML, got %d", code)
	}
}

func TestCatalogCheck_GoodEntry_PASS(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "src.md")
	os.WriteFile(sourceFile, []byte("content"), 0o644)
	targetFile := filepath.Join(tmp, "target.bin")
	os.WriteFile(targetFile, []byte("binary"), 0o644)

	reg := writeTempRegistry(t, tmp, `entries:
  - name: good
    bins: [ls]
    source: `+sourceFile+`
    targets:
      - `+targetFile+`
    verify_commands:
      - "ls --help"
`)

	output := captureStdout(t, func() {
		code := runCatalogCheck(context.Background(), []string{"--registry", reg, "--json"})
		if code != 0 {
			t.Errorf("expected exit 0, got %d", code)
		}
	})

	var results []catalogResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &results); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(results) != 1 || results[0].Status != "PASS" {
		t.Errorf("expected single PASS result, got %+v", results)
	}
}

func TestCatalogCheck_MissingBinary_BROKEN(t *testing.T) {
	tmp := t.TempDir()
	reg := writeTempRegistry(t, tmp, `entries:
  - name: missing-bin
    bins: [nonexistent-binary-xyz-abc]
`)

	output := captureStdout(t, func() {
		code := runCatalogCheck(context.Background(), []string{"--registry", reg, "--json"})
		if code != 1 {
			t.Errorf("expected exit 1, got %d", code)
		}
	})

	var results []catalogResult
	json.Unmarshal([]byte(strings.TrimSpace(output)), &results)
	if len(results) != 1 || results[0].Status != "BROKEN" {
		t.Errorf("expected BROKEN, got %+v", results)
	}
	if len(results[0].Details) == 0 || !strings.Contains(results[0].Details[0], "not on PATH") {
		t.Errorf("expected PATH detail, got %v", results[0].Details)
	}
}

func TestCatalogCheck_MissingSource_STALE(t *testing.T) {
	tmp := t.TempDir()
	reg := writeTempRegistry(t, tmp, `entries:
  - name: stale-src
    bins: [ls]
    source: /tmp/nonexistent-source-catalog-test-xyz
`)

	output := captureStdout(t, func() {
		runCatalogCheck(context.Background(), []string{"--registry", reg, "--json"})
	})

	var results []catalogResult
	json.Unmarshal([]byte(strings.TrimSpace(output)), &results)
	if len(results) != 1 || results[0].Status != "STALE" {
		t.Errorf("expected STALE, got %+v", results)
	}
}

func TestCatalogCheck_CommandTimeout_BROKEN(t *testing.T) {
	tmp := t.TempDir()
	reg := writeTempRegistry(t, tmp, `entries:
  - name: slow-cmd
    bins: [sleep]
    verify_commands:
      - "sleep 60"
`)

	// Use a very short parent context to force timeout quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	output := captureStdout(t, func() {
		code := runCatalogCheck(ctx, []string{"--registry", reg, "--json"})
		if code != 1 {
			t.Errorf("expected exit 1, got %d", code)
		}
	})

	var results []catalogResult
	json.Unmarshal([]byte(strings.TrimSpace(output)), &results)
	if len(results) != 1 || results[0].Status != "BROKEN" {
		t.Fatalf("expected BROKEN, got %+v", results)
	}
	found := false
	for _, d := range results[0].Details {
		if strings.Contains(d, "timed out") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected timeout detail, got %v", results[0].Details)
	}
}

func TestCatalogCheck_EmptyName_Skipped(t *testing.T) {
	tmp := t.TempDir()
	reg := writeTempRegistry(t, tmp, `entries:
  - name: ""
    bins: [ls]
  - name: valid
    bins: [ls]
`)

	output := captureStdout(t, func() {
		runCatalogCheck(context.Background(), []string{"--registry", reg, "--json"})
	})

	var results []catalogResult
	json.Unmarshal([]byte(strings.TrimSpace(output)), &results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (empty name skipped), got %d: %+v", len(results), results)
	}
	if results[0].Name != "valid" {
		t.Errorf("expected 'valid', got %q", results[0].Name)
	}
}

func TestCatalogCheck_JSONOutputFormat(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "source.md")
	os.WriteFile(sourceFile, []byte("content"), 0o644)

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
	if len(byName["broken-entry"].Details) == 0 {
		t.Error("broken-entry: expected details about missing binary")
	}
	if len(byName["stale-entry"].Details) == 0 {
		t.Error("stale-entry: expected details about missing source")
	}
}

func TestCatalogCheck_MixedStatuses(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "good-source.md")
	os.WriteFile(sourceFile, []byte("content"), 0o644)

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

func TestCatalogCheck_AllPass_ExitZero(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "source.md")
	os.WriteFile(sourceFile, []byte("content"), 0o644)

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

func TestCatalogCheck_EnvOverride(t *testing.T) {
	tmp := t.TempDir()
	sourceFile := filepath.Join(tmp, "source.md")
	os.WriteFile(sourceFile, []byte("content"), 0o644)

	reg := writeTempRegistry(t, tmp, `entries:
  - name: env-test
    bins: [ls]
    source: `+sourceFile+`
`)

	t.Setenv("GATE_REGISTRY", reg)

	output := captureStdout(t, func() {
		// No --registry flag — should pick up GATE_REGISTRY env var
		code := runCatalogCheck(context.Background(), []string{"--json"})
		if code != 0 {
			t.Errorf("expected exit 0, got %d", code)
		}
	})

	var results []catalogResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &results); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(results) != 1 || results[0].Name != "env-test" {
		t.Errorf("expected env-test entry, got %+v", results)
	}
}

func TestDefaultRegistryPath_EnvOverride(t *testing.T) {
	t.Setenv("GATE_REGISTRY", "/custom/registry.yaml")
	if got := defaultRegistryPath(); got != "/custom/registry.yaml" {
		t.Errorf("defaultRegistryPath() = %q, want /custom/registry.yaml", got)
	}
}

func TestDefaultRegistryPath_Fallback(t *testing.T) {
	t.Setenv("GATE_REGISTRY", "")
	if got := defaultRegistryPath(); got != fallbackRegistryPath {
		t.Errorf("defaultRegistryPath() = %q, want %q", got, fallbackRegistryPath)
	}
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
