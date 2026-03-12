package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectTestSuite_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	cmd := DetectTestSuite(dir, nil)
	if cmd == nil {
		t.Fatal("expected go test detection")
	}
	if cmd[0] != "go" || cmd[1] != "test" {
		t.Fatalf("expected go test, got %v", cmd)
	}
}

func TestDetectTestSuite_Node(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)

	cmd := DetectTestSuite(dir, nil)
	if cmd == nil {
		t.Fatal("expected npm test detection")
	}
	if cmd[0] != "npm" {
		t.Fatalf("expected npm, got %v", cmd)
	}
}

func TestDetectTestSuite_Python(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(""), 0644)

	cmd := DetectTestSuite(dir, nil)
	if cmd == nil {
		t.Fatal("expected pytest detection")
	}
	if cmd[0] != "pytest" {
		t.Fatalf("expected pytest, got %v", cmd)
	}
}

func TestDetectTestSuite_Rust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(""), 0644)

	cmd := DetectTestSuite(dir, nil)
	if cmd == nil {
		t.Fatal("expected cargo test detection")
	}
	if cmd[0] != "cargo" {
		t.Fatalf("expected cargo, got %v", cmd)
	}
}

func TestDetectTestSuite_PythonSetupPy(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup.py"), []byte(""), 0644)

	cmd := DetectTestSuite(dir, nil)
	if cmd == nil {
		t.Fatal("expected pytest detection for setup.py")
	}
	if cmd[0] != "pytest" {
		t.Fatalf("expected pytest, got %v", cmd)
	}
}

func TestDetectTestSuite_Bats(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "smoke.bats"), []byte("@test 'works' { true; }"), 0644)

	cmd := DetectTestSuite(dir, nil)
	if cmd == nil {
		t.Fatal("expected bats detection")
	}
	if cmd[0] != "bats" {
		t.Fatalf("expected bats, got %v", cmd)
	}
}

func TestDetectTestSuite_None(t *testing.T) {
	dir := t.TempDir()
	cmd := DetectTestSuite(dir, nil)
	if cmd != nil {
		t.Fatalf("expected nil, got %v", cmd)
	}
}

func TestDetectLinters_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	linters := DetectLinters(dir, nil)
	if len(linters) == 0 {
		t.Fatal("expected go vet detection")
	}
	if linters[0].Name != "go vet" {
		t.Fatalf("expected 'go vet', got %q", linters[0].Name)
	}
}

func TestDetectLinters_Shell(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/bash\necho hi"), 0644)

	linters := DetectLinters(dir, nil)
	found := false
	for _, l := range linters {
		if l.Name == "shellcheck" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected shellcheck detection")
	}
}

func TestDetectLinters_None(t *testing.T) {
	dir := t.TempDir()
	linters := DetectLinters(dir, nil)
	if len(linters) != 0 {
		t.Fatalf("expected no linters, got %v", linters)
	}
}

func TestDetectLinters_ESLint(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"devDependencies":{"eslint":"^8.0.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	linters := DetectLinters(dir, nil)
	found := false
	for _, l := range linters {
		if l.Name == "eslint" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected eslint detection")
	}
}

func TestDetectLinters_NodeNoESLint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{}}`), 0644)

	linters := DetectLinters(dir, nil)
	for _, l := range linters {
		if l.Name == "eslint" {
			t.Fatal("should not detect eslint when not in deps")
		}
	}
}

func TestDetectLinters_PythonRuff_PyFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("print('hi')"), 0644)

	linters := DetectLinters(dir, nil)
	found := false
	for _, l := range linters {
		if l.Name == "ruff" {
			found = true
			if l.Cmd[0] != "ruff" || l.Cmd[1] != "check" {
				t.Fatalf("expected ruff check, got %v", l.Cmd)
			}
		}
	}
	if !found {
		t.Fatal("expected ruff detection for .py files")
	}
}

func TestDetectLinters_PythonRuff_SetupPy(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup.py"), []byte(""), 0644)

	linters := DetectLinters(dir, nil)
	found := false
	for _, l := range linters {
		if l.Name == "ruff" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ruff detection for setup.py")
	}
}

func TestDetectLinters_PythonRuff_PyprojectToml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(""), 0644)

	linters := DetectLinters(dir, nil)
	found := false
	for _, l := range linters {
		if l.Name == "ruff" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ruff detection for pyproject.toml")
	}
}

func TestDetectLinters_PythonRuff_SrcDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)

	linters := DetectLinters(dir, nil)
	found := false
	for _, l := range linters {
		if l.Name == "ruff" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ruff detection for src/ directory")
	}
}

func TestDetectLinters_NoPython_NoRuff(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	linters := DetectLinters(dir, nil)
	for _, l := range linters {
		if l.Name == "ruff" {
			t.Fatal("should not detect ruff in non-Python project")
		}
	}
}

func TestDetectTestSuite_ConfigOverride(t *testing.T) {
	cmd := DetectTestSuite(t.TempDir(), &Config{
		Check: CheckConfig{Test: []string{"bash", "-lc", "echo override"}},
	})
	if len(cmd) != 3 || cmd[0] != "bash" || cmd[2] != "echo override" {
		t.Fatalf("unexpected override command: %v", cmd)
	}
}

func TestDetectLinters_ConfigOverride(t *testing.T) {
	linters := DetectLinters(t.TempDir(), &Config{
		Check: CheckConfig{
			Lint: []LinterSpec{{Name: "shellcheck", Cmd: []string{"bash", "-lc", "shellcheck foo.sh"}}},
		},
	})
	if len(linters) != 1 {
		t.Fatalf("expected 1 configured linter, got %d", len(linters))
	}
	if linters[0].Name != "shellcheck" || linters[0].Cmd[2] != "shellcheck foo.sh" {
		t.Fatalf("unexpected configured linter: %+v", linters[0])
	}
}
