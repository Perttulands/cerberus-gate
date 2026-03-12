package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func readTruthsayerFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", "truthsayer", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(data)
}

func TestParseTruthsayerOutput_JSON(t *testing.T) {
	output := `{
  "version": "1",
  "summary": {
    "total": 6,
    "errors": 3,
    "warnings": 2,
    "info": 1
  }
}`
	f := parseTruthsayerOutput(output)
	if f.Errors != 3 || f.Warnings != 2 || f.Info != 1 {
		t.Errorf("got %+v, want errors=3 warnings=2 info=1", f)
	}
}

func TestParseTruthsayerOutput_JSONWithLeadingLogs(t *testing.T) {
	output := `INFO scanning...
{
  "summary": {
    "errors": 0,
    "warnings": 1,
    "info": 2
  }
}`

	f := parseTruthsayerOutput(output)
	if f.Errors != 0 || f.Warnings != 1 || f.Info != 2 {
		t.Errorf("got %+v, want errors=0 warnings=1 info=2", f)
	}
}

func TestParseTruthsayerOutput_ZeroErrors(t *testing.T) {
	output := `{"summary":{"errors":0,"warnings":0,"info":0}}`
	f := parseTruthsayerOutput(output)
	if f.Errors != 0 || f.Warnings != 0 || f.Info != 0 {
		t.Errorf("got %+v, want all zeros", f)
	}
}

func TestParseTruthsayerOutput_FallbackCounting(t *testing.T) {
	// No summary line — should fall back to counting prefixes
	output := `ERROR   something.bad
  file.go:1
ERROR   another.bad
  file.go:2
WARN    minor.issue
  file.go:3`

	f := parseTruthsayerOutput(output)
	if f.Errors != 2 {
		t.Errorf("expected 2 errors from fallback, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning from fallback, got %d", f.Warnings)
	}
}

func TestParseTruthsayerOutput_EmptyOutput(t *testing.T) {
	f := parseTruthsayerOutput("")
	if f.Errors != 0 || f.Warnings != 0 || f.Info != 0 {
		t.Errorf("expected all zeros for empty output, got %+v", f)
	}
}

func TestParseTruthsayerOutput_FullJSONWithFindings(t *testing.T) {
	// Real-world output: JSON object with both findings array and summary.
	output := `{
  "version": "1",
  "scan_time": "2026-02-25T20:19:07Z",
  "path": ".",
  "duration_ms": 21,
  "findings": [
    {
      "rule": "trace-gaps.no-stderr-capture",
      "severity": "error",
      "file": "cmd/gate/main.go",
      "line": 382,
      "message": "exec.Command used without stderr capture"
    },
    {
      "rule": "bad-defaults.magic-number",
      "severity": "warning",
      "file": "internal/gates/lint.go",
      "line": 63,
      "message": "Magic number used directly"
    },
    {
      "rule": "trace-gaps.long-function-no-log",
      "severity": "info",
      "file": "internal/gates/lint.go",
      "line": 20,
      "message": "Function DetectLinters has no logging"
    }
  ],
  "summary": {
    "total": 3,
    "errors": 1,
    "warnings": 1,
    "info": 1,
    "files_scanned": 19,
    "duration_ms": 21
  }
}`

	f := parseTruthsayerOutput(output)
	if f.Errors != 1 {
		t.Errorf("expected 1 error, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", f.Warnings)
	}
	if f.Info != 1 {
		t.Errorf("expected 1 info, got %d", f.Info)
	}
}

func TestParseTruthsayerOutput_FindingsArrayWithoutSummary(t *testing.T) {
	// Edge case: JSON has findings but zero summary (unlikely but defensive).
	output := `{
  "findings": [
    {"severity": "error", "message": "a"},
    {"severity": "error", "message": "b"},
    {"severity": "warning", "message": "c"}
  ],
  "summary": {
    "errors": 0,
    "warnings": 0,
    "info": 0
  }
}`

	f := parseTruthsayerOutput(output)
	// Summary is all zeros but findings exist, so we count from findings.
	if f.Errors != 2 {
		t.Errorf("expected 2 errors from findings, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning from findings, got %d", f.Warnings)
	}
}

func TestParseTruthsayerOutput_JSONWithTrailingText(t *testing.T) {
	// JSON followed by trailing text — decoder should stop at object boundary.
	output := `{
  "summary": {"errors": 5, "warnings": 3, "info": 10}
}
Some trailing log line
Another trailing line`

	f := parseTruthsayerOutput(output)
	if f.Errors != 5 || f.Warnings != 3 || f.Info != 10 {
		t.Errorf("got %+v, want errors=5 warnings=3 info=10", f)
	}
}

func TestParseTruthsayerOutput_BannerThenJSON(t *testing.T) {
	output := readTruthsayerFixture(t, "banner_then_json.txt")

	f := parseTruthsayerOutput(output)
	if f.Errors != 1 || f.Warnings != 1 || f.Info != 1 {
		t.Errorf("got %+v, want errors=1 warnings=1 info=1", f)
	}
}

func TestParseTruthsayerOutput_MalformedJSON(t *testing.T) {
	// Invalid JSON should fall through to text fallback.
	output := `{invalid json}
ERROR bad.thing
WARN minor.thing`

	f := parseTruthsayerOutput(output)
	if f.Errors != 1 {
		t.Errorf("expected 1 error from fallback, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning from fallback, got %d", f.Warnings)
	}
}

func TestParseTruthsayerOutput_FixtureTextSummary(t *testing.T) {
	output := readTruthsayerFixture(t, "text_summary.txt")

	f := parseTruthsayerOutput(output)
	if f.Errors != 2 || f.Warnings != 3 || f.Info != 4 {
		t.Errorf("got %+v, want errors=2 warnings=3 info=4", f)
	}
}
