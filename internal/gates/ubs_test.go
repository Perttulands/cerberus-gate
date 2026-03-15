package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func readUBSFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", "ubs", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(data)
}

func TestParseUBSOutput_WithErrors(t *testing.T) {
	output := `{
  "totals": {
    "critical": 2,
    "warning": 0,
    "info": 1
  }
}`

	f := parseUBSOutput(output)
	if f.Errors != 2 {
		t.Errorf("expected 2 errors, got %d", f.Errors)
	}
}

func TestParseUBSOutput_Clean(t *testing.T) {
	output := `{"totals":{"critical":0,"warning":0,"info":0}}`

	f := parseUBSOutput(output)
	if f.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", f.Errors)
	}
}

func TestParseUBSOutput_WithWarnings(t *testing.T) {
	output := "\u26a0 possible issue detected\n\u26a0 another warning\n\u2717 critical failure"

	f := parseUBSOutput(output)
	if f.Warnings != 2 {
		t.Errorf("expected 2 warnings, got %d", f.Warnings)
	}
	if f.Errors != 1 {
		t.Errorf("expected 1 error, got %d", f.Errors)
	}
}

func TestParseUBSOutput_Empty(t *testing.T) {
	f := parseUBSOutput("")
	if f.Errors != 0 || f.Warnings != 0 {
		t.Errorf("expected all zeros for empty output, got %+v", f)
	}
}

func TestParseUBSOutput_UsesSummaryCounts(t *testing.T) {
	output := `INFO preparing shadow workspace
{
  "totals": {
    "critical": 1,
    "warning": 2,
    "info": 67
  }
}`

	f := parseUBSOutput(output)
	if f.Errors != 1 {
		t.Errorf("expected 1 error from summary, got %d", f.Errors)
	}
	if f.Warnings != 2 {
		t.Errorf("expected 2 warnings from summary, got %d", f.Warnings)
	}
	if f.Info != 67 {
		t.Errorf("expected 67 info items from summary, got %d", f.Info)
	}
}

func TestParseUBSOutput_ZeroSummaryOverridesIcons(t *testing.T) {
	output := "{\"totals\":{\"critical\":0,\"warning\":0,\"info\":0}}\n\u2717 stale icon should be ignored\n\u26a0 stale icon should be ignored"

	f := parseUBSOutput(output)
	if f.Errors != 0 || f.Warnings != 0 || f.Info != 0 {
		t.Errorf("expected summary zeros, got %+v", f)
	}
}

func TestParseUBSOutput_FullJSONWithScanners(t *testing.T) {
	// Real-world output: JSON with scanners array and totals.
	output := `{
  "project": "/home/user/projects/gate",
  "timestamp": "2026-02-25 22:19:08",
  "scanners": [
    {
      "project": "/home/user/projects/gate",
      "timestamp": "2026-02-25T20:19:08Z",
      "files": 20,
      "critical": 0,
      "warning": 1,
      "info": 86,
      "version": "7.1",
      "language": "golang"
    }
  ],
  "totals": {
    "critical": 0,
    "warning": 1,
    "info": 86,
    "files": 20
  }
}`

	f := parseUBSOutput(output)
	if f.Errors != 0 {
		t.Errorf("expected 0 critical, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", f.Warnings)
	}
	if f.Info != 86 {
		t.Errorf("expected 86 info, got %d", f.Info)
	}
}

func TestParseUBSOutput_BannerThenJSON(t *testing.T) {
	output := readUBSFixture(t, "banner_then_json.txt")

	f := parseUBSOutput(output)
	if f.Errors != 3 {
		t.Errorf("expected 3 critical, got %d", f.Errors)
	}
	if f.Warnings != 2 {
		t.Errorf("expected 2 warnings, got %d", f.Warnings)
	}
	if f.Info != 50 {
		t.Errorf("expected 50 info, got %d", f.Info)
	}
}

func TestParseUBSOutput_ScannersWithoutTotals(t *testing.T) {
	output := readUBSFixture(t, "scanners_without_totals.txt")

	f := parseUBSOutput(output)
	// Totals are zero but scanners have data — we sum from scanners.
	if f.Errors != 1 {
		t.Errorf("expected 1 critical from scanners, got %d", f.Errors)
	}
	if f.Warnings != 3 {
		t.Errorf("expected 3 warnings from scanners, got %d", f.Warnings)
	}
	if f.Info != 15 {
		t.Errorf("expected 15 info from scanners, got %d", f.Info)
	}
}

func TestParseUBSOutput_JSONWithTrailingText(t *testing.T) {
	// JSON followed by trailing text — decoder should stop at object boundary.
	output := `{
  "totals": {"critical": 1, "warning": 0, "info": 5, "files": 3}
}
Cleanup: removed temp workspace`

	f := parseUBSOutput(output)
	if f.Errors != 1 || f.Warnings != 0 || f.Info != 5 {
		t.Errorf("got %+v, want critical=1 warning=0 info=5", f)
	}
}

func TestParseUBSOutput_MalformedJSON(t *testing.T) {
	// Invalid JSON should fall through to icon fallback.
	output := "{broken json\n\u2717 real failure\n\u26a0 real warning"

	f := parseUBSOutput(output)
	if f.Errors != 1 {
		t.Errorf("expected 1 error from icon fallback, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning from icon fallback, got %d", f.Warnings)
	}
}

func TestParseUBSOutput_TruncatedJSON(t *testing.T) {
	output := `{"totals":{"critical":3,"warn`
	f := parseUBSOutput(output)
	// Truncated JSON, no icons → all zeros
	if f.Errors != 0 || f.Warnings != 0 || f.Info != 0 {
		t.Errorf("truncated JSON should yield zeros, got %+v", f)
	}
}

func TestParseUBSOutput_WhitespaceOnly(t *testing.T) {
	f := parseUBSOutput("   \n\t\n  ")
	if f.Errors != 0 || f.Warnings != 0 || f.Info != 0 {
		t.Errorf("whitespace-only should yield zeros, got %+v", f)
	}
}

func TestParseUBSOutput_BinaryGarbage(t *testing.T) {
	output := "some\x00binary\xffgarbage\x01here"
	f := parseUBSOutput(output)
	_ = f // should not panic
}

func TestParseUBSOutput_MissingSummaryFields(t *testing.T) {
	// Valid JSON with only critical, missing warning/info.
	output := `{"totals": {"critical": 4}}`
	f := parseUBSOutput(output)
	if f.Errors != 4 {
		t.Errorf("expected 4 critical, got %d", f.Errors)
	}
	if f.Warnings != 0 || f.Info != 0 {
		t.Errorf("missing fields should default to 0, got %+v", f)
	}
}

func TestParseUBSOutput_EmptyJSONObject(t *testing.T) {
	output := `{}`
	f := parseUBSOutput(output)
	if f.Errors != 0 || f.Warnings != 0 || f.Info != 0 {
		t.Errorf("empty JSON object should yield zeros, got %+v", f)
	}
}

func TestParseUBSOutput_MultipleJSONObjects(t *testing.T) {
	output := `{"totals":{"critical":2,"warning":1,"info":0}}
{"totals":{"critical":99,"warning":99,"info":99}}`
	f := parseUBSOutput(output)
	if f.Errors != 2 {
		t.Errorf("expected 2 critical from first object, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning from first object, got %d", f.Warnings)
	}
}

func TestParseUBSOutput_IconsWithNoJSON(t *testing.T) {
	// Pure icon output with no JSON at all.
	output := "\u2717 memory leak in handler.go:45\n\u2717 unclosed file in server.go:88\n\u26a0 shadowed variable in util.go:12\nclean: parser.go"
	f := parseUBSOutput(output)
	if f.Errors != 2 {
		t.Errorf("expected 2 criticals from icons, got %d", f.Errors)
	}
	if f.Warnings != 1 {
		t.Errorf("expected 1 warning from icons, got %d", f.Warnings)
	}
}
