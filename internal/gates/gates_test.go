package gates

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockRunCmd replaces runCmdFunc for the duration of the test.
func mockRunCmd(t *testing.T, fn func(ctx context.Context, dir string, timeoutSec int, name string, args ...string) (bool, string, error)) {
	t.Helper()
	orig := runCmdFunc
	t.Cleanup(func() { runCmdFunc = orig })
	runCmdFunc = fn
}

// --- RunTests ---

func TestRunTests_NoSuiteDetected(t *testing.T) {
	dir := t.TempDir() // empty dir, no go.mod etc.
	r := RunTests(context.Background(), dir, 30, nil)
	if !r.Pass {
		t.Fatal("expected pass when no test suite detected")
	}
	if r.Output != "no test suite detected" {
		t.Fatalf("unexpected output: %s", r.Output)
	}
}

func TestRunTests_Pass(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		if name != "go" || args[0] != "test" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		if d != dir {
			t.Fatalf("unexpected dir: %s", d)
		}
		return true, "ok\ttest\t0.001s", nil
	})

	r := RunTests(context.Background(), dir, 30, nil)
	if !r.Pass {
		t.Fatal("expected pass")
	}
	if r.Name != "tests" {
		t.Fatalf("expected name 'tests', got %q", r.Name)
	}
}

func TestRunTests_Fail(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, "FAIL test 0.001s", nil
	})

	r := RunTests(context.Background(), dir, 30, nil)
	if r.Pass {
		t.Fatal("expected fail")
	}
}

func TestRunTests_Error(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, "", fmt.Errorf("exec go: file not found")
	})

	r := RunTests(context.Background(), dir, 30, nil)
	if r.Pass {
		t.Fatal("expected fail on exec error")
	}
}

func TestRunTests_DefaultTimeout(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	var capturedTimeout int
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		capturedTimeout = timeout
		return true, "ok", nil
	})

	RunTests(context.Background(), dir, 0, nil) // zero means use default
	if capturedTimeout != 120 {
		t.Fatalf("expected default timeout 120, got %d", capturedTimeout)
	}
}

// --- RunLint ---

func TestRunLint_NoLintersDetected(t *testing.T) {
	dir := t.TempDir()
	results := RunLint(context.Background(), dir, 30, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Pass {
		t.Fatal("expected pass when no linters detected")
	}
	if results[0].Output != "no linters detected" {
		t.Fatalf("unexpected output: %s", results[0].Output)
	}
}

func TestRunLint_GoVetPass(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		if name != "go" || args[0] != "vet" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		return true, "", nil
	})

	results := RunLint(context.Background(), dir, 30, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Pass {
		t.Fatal("expected pass")
	}
	if results[0].Name != "lint:go vet" {
		t.Fatalf("expected name 'lint:go vet', got %q", results[0].Name)
	}
}

func TestRunLint_GoVetFail(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, "main.go:10: unreachable code", nil
	})

	results := RunLint(context.Background(), dir, 30, nil)
	if results[0].Pass {
		t.Fatal("expected fail")
	}
}

func TestRunLint_MultipleLinters(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/bash\necho hi"), 0644)

	var cmds []string
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		cmds = append(cmds, name)
		return true, "", nil
	})

	results := RunLint(context.Background(), dir, 30, nil)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results (go vet + shellcheck), got %d", len(results))
	}
	if len(cmds) < 2 {
		t.Fatalf("expected at least 2 commands, got %d", len(cmds))
	}
}

func TestRunLint_DefaultTimeout(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	var capturedTimeout int
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		capturedTimeout = timeout
		return true, "", nil
	})

	RunLint(context.Background(), dir, 0, nil)
	if capturedTimeout != 60 {
		t.Fatalf("expected default timeout 60, got %d", capturedTimeout)
	}
}

func TestRunLint_Error(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, "", fmt.Errorf("exec go: command not found")
	})

	results := RunLint(context.Background(), dir, 30, nil)
	if results[0].Pass {
		t.Fatal("expected fail on exec error")
	}
}

// --- RunTruthsayer ---

func TestRunTruthsayer_NotAvailable(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, "", fmt.Errorf("exec truthsayer: executable file not found")
	})

	r := RunTruthsayer(context.Background(), t.TempDir(), 30)
	if !r.Pass {
		t.Fatal("expected pass when truthsayer not available")
	}
	if !r.Skipped {
		t.Fatal("expected skipped=true")
	}
	if r.Name != "truthsayer" {
		t.Fatalf("expected name 'truthsayer', got %q", r.Name)
	}
}

func TestRunTruthsayer_CleanScan(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		if name != "truthsayer" {
			t.Fatalf("expected truthsayer, got %s", name)
		}
		return true, `{"findings":[],"summary":{"errors":0,"warnings":0,"info":0}}`, nil
	})

	r := RunTruthsayer(context.Background(), t.TempDir(), 30)
	if !r.Pass {
		t.Fatal("expected pass on clean scan")
	}
	if r.Skipped {
		t.Fatal("should not be skipped")
	}
	if r.Findings == nil {
		t.Fatal("expected findings to be set")
	}
	if r.Findings.Errors != 0 {
		t.Fatalf("expected 0 errors, got %d", r.Findings.Errors)
	}
}

func TestRunTruthsayer_WithErrors(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, `{"findings":[{"severity":"error"},{"severity":"warning"}],"summary":{"errors":1,"warnings":1,"info":0}}`, nil
	})

	r := RunTruthsayer(context.Background(), t.TempDir(), 30)
	if r.Pass {
		t.Fatal("expected fail when errors found")
	}
	if r.Findings.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", r.Findings.Errors)
	}
	if r.Findings.Warnings != 1 {
		t.Fatalf("expected 1 warning, got %d", r.Findings.Warnings)
	}
}

func TestRunTruthsayer_CmdFailWithNoErrors(t *testing.T) {
	// Command exits non-zero but output has zero errors — still fails because cmdPass is false
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, `{"findings":[],"summary":{"errors":0,"warnings":0,"info":0}}`, nil
	})

	r := RunTruthsayer(context.Background(), t.TempDir(), 30)
	if r.Pass {
		t.Fatal("expected fail when cmd exits non-zero even with zero error findings")
	}
}

func TestRunTruthsayerCI_UsesCICommand(t *testing.T) {
	var called bool
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		called = true
		if name != "truthsayer" {
			t.Fatalf("expected truthsayer, got %s", name)
		}
		expected := []string{"ci", "."}
		if len(args) != len(expected) {
			t.Fatalf("expected args %v, got %v", expected, args)
		}
		for i, a := range args {
			if a != expected[i] {
				t.Fatalf("arg[%d]: expected %q, got %q", i, expected[i], a)
			}
		}
		return true, "Summary: 0 errors, 0 warnings, 0 info (2 files scanned in 13ms)", nil
	})

	r := RunTruthsayerCI(context.Background(), t.TempDir(), 30, nil)
	if !called {
		t.Fatal("expected runCmd to be called")
	}
	if !r.Pass {
		t.Fatal("expected pass")
	}
}

func TestRunTruthsayer_DefaultTimeout(t *testing.T) {
	var capturedTimeout int
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		capturedTimeout = timeout
		return true, `{"findings":[],"summary":{"errors":0,"warnings":0,"info":0}}`, nil
	})

	RunTruthsayer(context.Background(), t.TempDir(), 0)
	if capturedTimeout != 60 {
		t.Fatalf("expected default timeout 60, got %d", capturedTimeout)
	}
}

func TestRunTruthsayer_CorrectArgs(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		expected := []string{"scan", ".", "--format", "json"}
		if len(args) != len(expected) {
			t.Fatalf("expected args %v, got %v", expected, args)
		}
		for i, a := range args {
			if a != expected[i] {
				t.Fatalf("arg[%d]: expected %q, got %q", i, expected[i], a)
			}
		}
		return true, `{"findings":[],"summary":{"errors":0,"warnings":0,"info":0}}`, nil
	})

	RunTruthsayer(context.Background(), t.TempDir(), 30)
}

// --- RunUBS ---

func TestRunUBS_NotAvailable(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, "", fmt.Errorf("exec ubs: executable file not found")
	})

	r := RunUBS(context.Background(), t.TempDir(), 30)
	if !r.Pass {
		t.Fatal("expected pass when ubs not available")
	}
	if !r.Skipped {
		t.Fatal("expected skipped=true")
	}
}

func TestRunUBS_CleanScan(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		if name != "ubs" {
			t.Fatalf("expected ubs, got %s", name)
		}
		return true, `{"scanners":[],"totals":{"critical":0,"warning":0,"info":0,"files":5}}`, nil
	})

	r := RunUBS(context.Background(), t.TempDir(), 30)
	if !r.Pass {
		t.Fatal("expected pass on clean scan")
	}
	if r.Findings == nil {
		t.Fatal("expected findings to be set")
	}
}

func TestRunUBS_WithCritical(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, `{"scanners":[],"totals":{"critical":2,"warning":1,"info":0,"files":10}}`, nil
	})

	r := RunUBS(context.Background(), t.TempDir(), 30)
	if r.Pass {
		t.Fatal("expected fail with critical findings")
	}
	if r.Findings.Errors != 2 {
		t.Fatalf("expected 2 errors, got %d", r.Findings.Errors)
	}
}

func TestRunUBS_FullScanArgs(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--diff") {
			t.Fatal("full scan should not include --diff")
		}
		if !strings.Contains(joined, "--format=json") {
			t.Fatal("expected --format=json")
		}
		return true, `{"scanners":[],"totals":{"critical":0,"warning":0,"info":0,"files":1}}`, nil
	})

	RunUBS(context.Background(), t.TempDir(), 30)
}

func TestRunUBSDiff_DiffArgs(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--diff") {
			t.Fatal("diff mode should include --diff")
		}
		return true, `{"scanners":[],"totals":{"critical":0,"warning":0,"info":0,"files":1}}`, nil
	})

	RunUBSDiff(context.Background(), t.TempDir(), 30, nil)
}

func TestRunUBSDiff_FallbackToFullScan(t *testing.T) {
	var callCount int
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		callCount++
		joined := strings.Join(args, " ")
		if callCount == 1 {
			// First call: diff mode fails (non-git context)
			if !strings.Contains(joined, "--diff") {
				t.Fatal("first call should be diff mode")
			}
			return false, "not a git repository", nil
		}
		// Second call: full scan succeeds
		if strings.Contains(joined, "--diff") {
			t.Fatal("fallback should not include --diff")
		}
		return true, `{"scanners":[],"totals":{"critical":0,"warning":0,"info":0,"files":1}}`, nil
	})

	r := RunUBSDiff(context.Background(), t.TempDir(), 30, nil)
	if callCount != 2 {
		t.Fatalf("expected 2 calls (diff + fallback), got %d", callCount)
	}
	if !r.Pass {
		t.Fatal("expected pass after fallback")
	}
}

func TestRunUBSDiff_DiffSucceeds(t *testing.T) {
	var callCount int
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		callCount++
		return true, `{"scanners":[],"totals":{"critical":0,"warning":0,"info":0,"files":1}}`, nil
	})

	r := RunUBSDiff(context.Background(), t.TempDir(), 30, nil)
	if callCount != 1 {
		t.Fatalf("expected 1 call (diff succeeded), got %d", callCount)
	}
	if !r.Pass {
		t.Fatal("expected pass")
	}
}

func TestRunUBS_DefaultTimeout(t *testing.T) {
	var capturedTimeout int
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		capturedTimeout = timeout
		return true, `{"scanners":[],"totals":{"critical":0,"warning":0,"info":0,"files":1}}`, nil
	})

	RunUBS(context.Background(), t.TempDir(), 0)
	if capturedTimeout != 60 {
		t.Fatalf("expected default timeout 60, got %d", capturedTimeout)
	}
}

func TestRunUBS_CmdFailWithNoCritical(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return false, `{"scanners":[],"totals":{"critical":0,"warning":0,"info":0,"files":1}}`, nil
	})

	r := RunUBS(context.Background(), t.TempDir(), 30)
	if r.Pass {
		t.Fatal("expected fail when cmd exits non-zero even with zero critical findings")
	}
}

func TestRunUBS_OutputSummaryFormat(t *testing.T) {
	mockRunCmd(t, func(ctx context.Context, d string, timeout int, name string, args ...string) (bool, string, error) {
		return true, `{"scanners":[],"totals":{"critical":0,"warning":3,"info":1,"files":10}}`, nil
	})

	r := RunUBS(context.Background(), t.TempDir(), 30)
	if !r.Pass {
		t.Fatal("expected pass")
	}
	if !strings.Contains(r.Output, "critical=0") {
		t.Fatalf("expected output to contain critical=0, got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "warning=3") {
		t.Fatalf("expected output to contain warning=3, got: %s", r.Output)
	}
}
