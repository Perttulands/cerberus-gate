package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"polis/gate/internal/city"
	"polis/gate/internal/verdict"
)

func TestValidateFilterValue(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid simple", "--repo", "gate", "gate", false},
		{"valid with dots", "--repo", "my.repo", "my.repo", false},
		{"valid with dash", "--citizen", "john-doe", "john-doe", false},
		{"valid with underscore", "--repo", "my_repo", "my_repo", false},
		{"trims whitespace", "--repo", "  gate  ", "gate", false},
		{"empty after trim", "--repo", "   ", "", true},
		{"empty string", "--repo", "", "", true},
		{"invalid chars slash", "--repo", "foo/bar", "", true},
		{"invalid chars space", "--repo", "foo bar", "", true},
		{"invalid chars colon", "--citizen", "foo:bar", "", true},
		{"invalid chars at", "--repo", "user@host", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateFilterValue(tt.flag, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateFilterValue(%q, %q) error = %v, wantErr %v", tt.flag, tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("validateFilterValue(%q, %q) = %q, want %q", tt.flag, tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveCitizen(t *testing.T) {
	t.Run("explicit value wins", func(t *testing.T) {
		got := resolveCitizen("alice", "")
		if got != "alice" {
			t.Fatalf("got %q, want %q", got, "alice")
		}
	})

	t.Run("trims explicit whitespace", func(t *testing.T) {
		got := resolveCitizen("  bob  ", "")
		if got != "bob" {
			t.Fatalf("got %q, want %q", got, "bob")
		}
	})

	t.Run("env var when explicit empty", func(t *testing.T) {
		t.Setenv("POLIS_CITIZEN", "env-user")
		got := resolveCitizen("", "")
		if got != "env-user" {
			t.Fatalf("got %q, want %q", got, "env-user")
		}
	})

	t.Run("env var trimmed", func(t *testing.T) {
		t.Setenv("POLIS_CITIZEN", "  spaced  ")
		got := resolveCitizen("", "")
		if got != "spaced" {
			t.Fatalf("got %q, want %q", got, "spaced")
		}
	})

	t.Run("git config is resolved from target repo", func(t *testing.T) {
		targetRepo := t.TempDir()
		cwdRepo := t.TempDir()
		mustRunGit(t, targetRepo, "init")
		mustRunGit(t, targetRepo, "config", "user.email", "target@example.com")
		mustRunGit(t, targetRepo, "config", "user.name", "target-user")
		mustRunGit(t, cwdRepo, "init")
		mustRunGit(t, cwdRepo, "config", "user.email", "cwd@example.com")
		mustRunGit(t, cwdRepo, "config", "user.name", "cwd-user")

		oldWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(cwdRepo); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		defer func() {
			if err := os.Chdir(oldWd); err != nil {
				t.Fatalf("restore cwd: %v", err)
			}
		}()

		t.Setenv("POLIS_CITIZEN", "")
		got := resolveCitizen("", targetRepo)
		if got != "target-user" {
			t.Fatalf("got %q, want %q", got, "target-user")
		}
	})

	t.Run("falls back to unknown when no explicit env or git identity exists", func(t *testing.T) {
		repo := t.TempDir()
		mustRunGit(t, repo, "init")
		emptyGlobal := filepath.Join(t.TempDir(), "empty.gitconfig")
		if err := os.WriteFile(emptyGlobal, nil, 0o644); err != nil {
			t.Fatalf("write empty global config: %v", err)
		}

		t.Setenv("POLIS_CITIZEN", "")
		t.Setenv("HOME", t.TempDir())
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
		t.Setenv("GIT_CONFIG_GLOBAL", emptyGlobal)

		got := resolveCitizen("", repo)
		if got != "unknown" {
			t.Fatalf("got %q, want %q", got, "unknown")
		}
	})
}

func TestRun_Help(t *testing.T) {
	// Redirect stdout to discard help output
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = old }()

	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"help", []string{"help"}},
		{"--help", []string{"--help"}},
		{"-h", []string{"-h"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := run(nil, tt.args)
			if code != 0 {
				t.Fatalf("run(%v) = %d, want 0", tt.args, code)
			}
		})
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = old }()

	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	code := run(nil, []string{"bogus"})
	if code != 1 {
		t.Fatalf("run(bogus) = %d, want 1", code)
	}
}

func TestRunCheck_MissingRepo(t *testing.T) {
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	code := runCheck(nil, []string{})
	if code != 1 {
		t.Fatalf("runCheck with no repo = %d, want 1", code)
	}
}

func TestRunCheck_FlagErrors(t *testing.T) {
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	tests := []struct {
		name string
		args []string
	}{
		{"--level without value", []string{"--level"}},
		{"--citizen without value", []string{"--citizen"}},
		{"unknown flag", []string{"--bogus", "."}},
		{"invalid level", []string{"--level", "extreme", "."}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := runCheck(nil, tt.args)
			if code != 1 {
				t.Fatalf("runCheck(%v) = %d, want 1", tt.args, code)
			}
		})
	}
}

func TestRunCity_MissingRepo(t *testing.T) {
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	code := runCity(nil, []string{})
	if code != 3 {
		t.Fatalf("runCity with no repo = %d, want 3 (ExitInvalid)", code)
	}
}

func TestRunCity_FlagErrors(t *testing.T) {
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"--install-at without value", []string{"--install-at"}, 3},
		{"--standalone-timeout without value", []string{"--standalone-timeout"}, 3},
		{"--standalone-timeout invalid", []string{"--standalone-timeout", "nope", "."}, 3},
		{"--standalone-timeout negative", []string{"--standalone-timeout", "-5s", "."}, 3},
		{"--citizen without value", []string{"--citizen"}, 3},
		{"unknown flag", []string{"--bogus", "."}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := runCity(nil, tt.args)
			if code != tt.want {
				t.Fatalf("runCity(%v) = %d, want %d", tt.args, code, tt.want)
			}
		})
	}
}

// --- helpers ---

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldOut := os.Stdout
	oldErr := os.Stderr
	r, w, _ := os.Pipe()
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = w
	os.Stderr = devNull
	fn()
	w.Close()
	devNull.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func writeTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	target := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
	}
}

func buildGateBinary(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	bin := filepath.Join(t.TempDir(), "gate-test")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = wd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build gate binary failed: %v (%s)", err, string(out))
	}
	return bin
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func runGateBinary(t *testing.T, bin string, env []string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out)
	}

	t.Fatalf("run gate binary %v failed: %v (%s)", args, err, string(out))
	return 0, ""
}

func runGateBinaryInDir(t *testing.T, bin, dir string, env []string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out)
	}

	t.Fatalf("run gate binary in %s %v failed: %v (%s)", dir, args, err, string(out))
	return 0, ""
}

func contractPathEnv(t *testing.T, extraDir string) []string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("lookpath go: %v", err)
	}

	pathValue := strings.Join([]string{extraDir, filepath.Dir(goBin), "/usr/bin", "/bin"}, string(os.PathListSeparator))
	return []string{
		"PATH=" + pathValue,
		"POLIS_CITIZEN=contract-user",
		"HOME=" + t.TempDir(),
	}
}

func gateByName(t *testing.T, gates []verdict.GateResult, name string) verdict.GateResult {
	t.Helper()
	for _, g := range gates {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("missing gate %q in %+v", name, gates)
	return verdict.GateResult{}
}

// --- E2E: runCheck ---

func TestRunCheck_E2E_PassingGoProject(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module passing\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	output := captureStdout(t, func() {
		code := runCheck(context.Background(), []string{"--level", "quick", "--json", dir})
		if code != 0 {
			t.Errorf("expected exit 0, got %d", code)
		}
	})

	var v verdict.Verdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &v); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, output)
	}
	if !v.Pass {
		t.Errorf("expected pass, got fail: %+v", v)
	}
	if v.Level != "quick" {
		t.Errorf("expected level quick, got %q", v.Level)
	}
	if v.Score <= 0 {
		t.Errorf("expected positive score, got %f", v.Score)
	}
}

func TestRunCheck_E2E_FailingGoProject(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module failing\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", "package main\nfunc main() {}\n")
	writeTestFile(t, dir, "fail_test.go", "package main\nimport \"testing\"\nfunc TestFail(t *testing.T) { t.Fatal(\"intentional\") }\n")

	output := captureStdout(t, func() {
		code := runCheck(context.Background(), []string{"--level", "quick", "--json", dir})
		if code != 1 {
			t.Errorf("expected exit 1, got %d", code)
		}
	})

	var v verdict.Verdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &v); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if v.Pass {
		t.Error("expected fail")
	}
	if v.ExitCode != 1 {
		t.Errorf("expected exit_code 1, got %d", v.ExitCode)
	}
}

func TestRunCheck_E2E_PrettyOutput(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module prettytest\n\ngo 1.21\n")
	writeTestFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	output := captureStdout(t, func() {
		code := runCheck(context.Background(), []string{"--level", "quick", "--citizen", "test-user", dir})
		if code != 0 {
			t.Errorf("expected exit 0, got %d", code)
		}
	})

	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS in output, got: %s", output)
	}
	if !strings.Contains(output, "quick") {
		t.Errorf("expected level in output")
	}
	if !strings.Contains(output, "test-user") {
		t.Errorf("expected citizen in output")
	}
}

func TestRunCheck_E2E_BinaryStandardScannerContract(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, repo, "go.mod", "module contractrepo\n\ngo 1.21\n")
	writeTestFile(t, repo, "main.go", "package main\nfunc main() {}\n")
	writeTestFile(t, repo, "main_test.go", "package main\nimport \"testing\"\nfunc TestPass(t *testing.T) {}\n")

	fakeBin := t.TempDir()
	invocations := filepath.Join(t.TempDir(), "scanners.log")
	writeExecutable(t, filepath.Join(fakeBin, "truthsayer"), `#!/bin/sh
printf 'truthsayer:%s\n' "$*" >> "`+invocations+`"
cat <<'EOF'
{"findings":[],"summary":{"errors":0,"warnings":1,"info":2}}
EOF
`)
	writeExecutable(t, filepath.Join(fakeBin, "ubs"), `#!/bin/sh
printf 'ubs:%s\n' "$*" >> "`+invocations+`"
cat <<'EOF'
{"scanners":[],"totals":{"critical":0,"warning":2,"info":3,"files":4}}
EOF
`)

	code, output := runGateBinary(
		t,
		buildGateBinary(t),
		contractPathEnv(t, fakeBin),
		"check", repo, "--level", "standard", "--json",
	)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (%s)", code, output)
	}

	var v verdict.Verdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &v); err != nil {
		t.Fatalf("parse gate JSON: %v\nraw: %s", err, output)
	}
	if !v.Pass {
		t.Fatalf("expected pass verdict, got %+v", v)
	}

	truthsayer := gateByName(t, v.Gates, "truthsayer")
	if truthsayer.Findings == nil || truthsayer.Findings.Warnings != 1 || truthsayer.Findings.Info != 2 {
		t.Fatalf("unexpected truthsayer findings: %+v", truthsayer.Findings)
	}

	ubs := gateByName(t, v.Gates, "ubs")
	if ubs.Findings == nil || ubs.Findings.Warnings != 2 || ubs.Findings.Info != 3 {
		t.Fatalf("unexpected ubs findings: %+v", ubs.Findings)
	}

	logData, err := os.ReadFile(invocations)
	if err != nil {
		t.Fatalf("read scanner log: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "truthsayer:ci .") {
		t.Fatalf("expected truthsayer ci contract, got %q", logText)
	}
	if !strings.Contains(logText, "ubs:--diff --format=json .") {
		t.Fatalf("expected ubs diff contract, got %q", logText)
	}
}

func TestRunCheck_E2E_BinaryJSONHealthContract_Failure(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, repo, "go.mod", "module healthcontract\n\ngo 1.21\n")
	writeTestFile(t, repo, "main.go", "package main\nfunc main() {}\n")
	writeTestFile(t, repo, "main_test.go", "package main\nimport \"testing\"\nfunc TestFail(t *testing.T) { t.Fatal(\"boom\") }\n")

	code, output := runGateBinary(
		t,
		buildGateBinary(t),
		contractPathEnv(t, t.TempDir()),
		"check", repo, "--level", "quick", "--json",
	)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d (%s)", code, output)
	}

	var v verdict.Verdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &v); err != nil {
		t.Fatalf("parse gate JSON: %v\nraw: %s", err, output)
	}
	if v.Pass {
		t.Fatalf("expected failing verdict, got %+v", v)
	}
	if v.ExitCode != 1 {
		t.Fatalf("expected exit_code 1, got %d", v.ExitCode)
	}
	if v.Level != "quick" {
		t.Fatalf("expected level quick, got %q", v.Level)
	}

	testsGate := gateByName(t, v.Gates, "tests")
	if testsGate.Pass {
		t.Fatalf("expected tests gate to fail, got %+v", testsGate)
	}
}

func TestRunHealth_E2E_BinaryDefaultRepoContract(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, repo, "go.mod", "module healthpass\n\ngo 1.21\n")
	writeTestFile(t, repo, "main.go", "package main\nfunc main() {}\n")

	code, output := runGateBinaryInDir(
		t,
		buildGateBinary(t),
		repo,
		contractPathEnv(t, t.TempDir()),
		"health",
	)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (%s)", code, output)
	}
	if strings.TrimSpace(output) != "healthy" {
		t.Fatalf("expected healthy output, got %q", output)
	}
}

func TestRunHealth_E2E_JSONFailureContract(t *testing.T) {
	repo := t.TempDir()
	writeTestFile(t, repo, "go.mod", "module healthfail\n\ngo 1.21\n")
	writeTestFile(t, repo, "main.go", "package main\nfunc main() {}\n")
	writeTestFile(t, repo, "main_test.go", "package main\nimport \"testing\"\nfunc TestFail(t *testing.T) { t.Fatal(\"boom\") }\n")

	code, output := runGateBinary(
		t,
		buildGateBinary(t),
		contractPathEnv(t, t.TempDir()),
		"health", repo, "--json",
	)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d (%s)", code, output)
	}

	var v verdict.Verdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &v); err != nil {
		t.Fatalf("parse gate health JSON: %v\nraw: %s", err, output)
	}
	if v.Pass {
		t.Fatalf("expected failing health verdict, got %+v", v)
	}
	if v.Level != "quick" {
		t.Fatalf("expected level quick, got %q", v.Level)
	}
	testsGate := gateByName(t, v.Gates, "tests")
	if testsGate.Pass {
		t.Fatalf("expected tests gate to fail, got %+v", testsGate)
	}
}

// --- E2E: runCity ---

func TestRunCity_E2E_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".gitignore", "polis.yaml\n")
	writeTestFile(t, dir, "city.toml", "[city]\nschema_version = 1\npolis_files = [\"polis.yaml\"]\nstandalone_check = \"\"\n")
	mustRunGit(t, dir, "init")
	mustRunGit(t, dir, "config", "user.email", "test@example.com")
	mustRunGit(t, dir, "config", "user.name", "test")
	mustRunGit(t, dir, "add", ".")
	mustRunGit(t, dir, "commit", "-m", "init")

	output := captureStdout(t, func() {
		code := runCity(context.Background(), []string{"--skip-standalone", "--json", dir})
		// exit 0=pass, 2=warn (skipped checks cause warn)
		if code != 0 && code != 2 {
			t.Errorf("expected exit 0 or 2, got %d", code)
		}
	})

	var v city.Verdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &v); err != nil {
		t.Fatalf("failed to parse JSON: %v\nraw: %s", err, output)
	}
	if v.Repo == "" {
		t.Error("expected repo name in verdict")
	}
	if len(v.Checks) == 0 {
		t.Error("expected at least one check result")
	}
}

// --- printPretty ---

func TestPrintPretty_PassVerdict(t *testing.T) {
	v := verdict.Verdict{
		Pass:    true,
		Score:   1.0,
		Level:   "quick",
		Repo:    "test-repo",
		Citizen: "tester",
		Gates: []verdict.GateResult{
			{Name: "tests", Pass: true, DurationMs: 42},
			{Name: "lint:go vet", Pass: true, DurationMs: 10},
		},
	}

	output := captureStdout(t, func() { printPretty(v) })

	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS in output, got: %s", output)
	}
	if !strings.Contains(output, "test-repo") {
		t.Errorf("expected repo name in output")
	}
	if !strings.Contains(output, "quick") {
		t.Errorf("expected level in output")
	}
	if !strings.Contains(output, "tester") {
		t.Errorf("expected citizen in output")
	}
	if !strings.Contains(output, "tests") {
		t.Errorf("expected gate name in output")
	}
}

func TestPrintPretty_FailVerdictShowsOutput(t *testing.T) {
	v := verdict.Verdict{
		Pass:  false,
		Score: 0.5,
		Level: "standard",
		Repo:  "fail-repo",
		Gates: []verdict.GateResult{
			{Name: "tests", Pass: true, DurationMs: 42},
			{Name: "lint:go vet", Pass: false, Output: "main.go:10: unreachable code", DurationMs: 10},
		},
	}

	output := captureStdout(t, func() { printPretty(v) })

	if !strings.Contains(output, "FAIL") {
		t.Errorf("expected FAIL in output, got: %s", output)
	}
	if !strings.Contains(output, "unreachable code") {
		t.Errorf("expected failure detail in output, got: %s", output)
	}
}

func TestPrintPretty_SkippedGate(t *testing.T) {
	v := verdict.Verdict{
		Pass: true,
		Repo: "skip-repo",
		Gates: []verdict.GateResult{
			{Name: "tests", Pass: true, DurationMs: 10},
			{Name: "truthsayer", Pass: true, Skipped: true, Output: "not available", DurationMs: 0},
		},
	}

	output := captureStdout(t, func() { printPretty(v) })

	if !strings.Contains(output, "truthsayer") {
		t.Errorf("expected skipped gate name in output")
	}
	// Skipped gate output should NOT be shown
	if strings.Contains(output, "not available") {
		t.Errorf("skipped gate output should not appear in pretty output")
	}
}

func TestPrintPretty_WithBeadID(t *testing.T) {
	v := verdict.Verdict{
		Pass: true,
		Repo: "bead-repo",
		Bead: "pol-42",
		Gates: []verdict.GateResult{
			{Name: "tests", Pass: true},
		},
	}

	output := captureStdout(t, func() { printPretty(v) })

	if !strings.Contains(output, "pol-42") {
		t.Errorf("expected bead ID in output, got: %s", output)
	}
}

// --- printPrettyCity ---

func TestPrintPrettyCity_PassVerdict(t *testing.T) {
	v := city.Verdict{
		Pass:   true,
		Status: "pass",
		Repo:   "pass-city",
		Checks: []city.CheckResult{
			{Name: "boundary", Status: city.StatusPass, Detail: "ok", DurationMs: 5},
		},
		Summary: city.Summary{Pass: 1},
	}

	output := captureStdout(t, func() { printPrettyCity(v) })

	if !strings.Contains(output, "PASS") {
		t.Errorf("expected PASS, got: %s", output)
	}
	if !strings.Contains(output, "pass-city") {
		t.Errorf("expected repo name")
	}
	if !strings.Contains(output, "pass=1") {
		t.Errorf("expected summary, got: %s", output)
	}
}

func TestPrintPrettyCity_WarnVerdict(t *testing.T) {
	v := city.Verdict{
		Status: "warn",
		Repo:   "warn-city",
		Checks: []city.CheckResult{
			{Name: "standalone", Status: city.StatusSkip, Detail: "skipped", DurationMs: 0},
		},
		Summary: city.Summary{Skip: 1},
	}

	output := captureStdout(t, func() { printPrettyCity(v) })

	if !strings.Contains(output, "WARN") {
		t.Errorf("expected WARN, got: %s", output)
	}
}

func TestPrintPrettyCity_FailVerdict(t *testing.T) {
	v := city.Verdict{
		Status: "fail",
		Repo:   "fail-city",
		Checks: []city.CheckResult{
			{Name: "boundary", Status: city.StatusFail, Detail: "not ignored", DurationMs: 3},
		},
		Summary: city.Summary{Fail: 1},
	}

	output := captureStdout(t, func() { printPrettyCity(v) })

	if !strings.Contains(output, "FAIL") {
		t.Errorf("expected FAIL, got: %s", output)
	}
	if !strings.Contains(output, "fail-city") {
		t.Errorf("expected repo name")
	}
	if !strings.Contains(output, "fail=1") {
		t.Errorf("expected summary, got: %s", output)
	}
}

func TestPrintPrettyCity_WithBeadID(t *testing.T) {
	v := city.Verdict{
		Status:  "pass",
		Repo:    "bead-city",
		Bead:    "pol-99",
		Summary: city.Summary{Pass: 1},
	}

	output := captureStdout(t, func() { printPrettyCity(v) })

	if !strings.Contains(output, "pol-99") {
		t.Errorf("expected bead ID, got: %s", output)
	}
}

func TestRunHistory_FlagErrors(t *testing.T) {
	oldErr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = oldErr }()

	tests := []struct {
		name string
		args []string
	}{
		{"--repo without value", []string{"--repo"}},
		{"--citizen without value", []string{"--citizen"}},
		{"--limit without value", []string{"--limit"}},
		{"--limit zero", []string{"--limit", "0"}},
		{"--limit negative", []string{"--limit", "-5"}},
		{"--limit non-numeric", []string{"--limit", "abc"}},
		{"unknown flag", []string{"--bogus"}},
		{"--repo invalid chars", []string{"--repo", "foo/bar"}},
		{"--citizen invalid chars", []string{"--citizen", "a b c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := runHistory(tt.args)
			if code != 1 {
				t.Fatalf("runHistory(%v) = %d, want 1", tt.args, code)
			}
		})
	}
}

func TestRunHistory_UsesSharedBRClient(t *testing.T) {
	tmp := t.TempDir()
	argsPath := filepath.Join(tmp, "br.args")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	brScript := filepath.Join(binDir, "br")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsPath + "\nprintf 'history-output\\n'\n"
	if err := os.WriteFile(brScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write br script: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	output := captureStdout(t, func() {
		code := runHistory([]string{"--repo", "relay", "--citizen", "alice", "--limit", "7"})
		if code != 0 {
			t.Fatalf("runHistory returned %d, want 0", code)
		}
	})

	if !strings.Contains(output, "history-output") {
		t.Fatalf("stdout = %q, want history output", output)
	}

	args := readLines(t, argsPath)
	want := []string{"search", "gate", "--type", "gate", "--sort", "created", "--reverse", "--limit", "7", "--label", "repo:relay", "--assignee", "alice"}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("history args = %#v, want %#v", args, want)
	}
}
