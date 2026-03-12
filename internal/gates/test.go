package gates

import (
	"context"
	"os"
	"path/filepath"

	"polis/gate/internal/verdict"
)

// DetectTestSuite returns the command and args to run tests for the repo at dir.
// Returns nil if no known test framework is detected.
func DetectTestSuite(dir string, cfg *Config) []string {
	if cfg != nil && len(cfg.Check.Test) > 0 {
		return cfg.Check.Test
	}

	// Go
	if fileExists(filepath.Join(dir, "go.mod")) {
		return []string{"go", "test", "./..."}
	}
	// Node
	if fileExists(filepath.Join(dir, "package.json")) {
		return []string{"npm", "test"}
	}
	// Python
	if fileExists(filepath.Join(dir, "pyproject.toml")) || fileExists(filepath.Join(dir, "setup.py")) {
		return []string{"pytest"}
	}
	// Rust
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		return []string{"cargo", "test"}
	}
	// Bats
	matches, err := filepath.Glob(filepath.Join(dir, "*.bats"))
	if err == nil && len(matches) > 0 {
		return []string{"bats", "."}
	}
	return nil
}

// RunTests detects and runs the test suite for the repo at dir.
func RunTests(ctx context.Context, dir string, timeoutSec int, cfg *Config) verdict.GateResult {
	cmd := DetectTestSuite(dir, cfg)
	if cmd == nil {
		return verdict.GateResult{Name: "tests", Pass: true, Output: "no test suite detected"}
	}
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	return verdict.TimedRun("tests", func() (bool, string, error) {
		pass, output, err := runCmd(ctx, dir, timeoutSec, cmd[0], cmd[1:]...)
		return pass, output, err
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
