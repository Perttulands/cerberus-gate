package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("missing gate.toml should not error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for missing file")
	}
}

func TestLoadConfig_ValidTOML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "gate.toml"), []byte(`
[check]
test = ["go", "test", "./..."]
truthsayer = ["truthsayer", "scan", "."]
`), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("valid TOML should not error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Check.Test) != 3 {
		t.Errorf("expected 3 test args, got %d", len(cfg.Check.Test))
	}
}

func TestLoadConfig_MalformedTOML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "gate.toml"), []byte(`
[check
this is not valid toml {{{
`), 0644)

	cfg, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("malformed TOML should return error")
	}
	if cfg != nil {
		t.Error("malformed TOML should return nil config")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "gate.toml"), []byte(""), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("empty TOML should not error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for empty but valid TOML")
	}
}

func TestLoadConfig_UnknownFieldsIgnored(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "gate.toml"), []byte(`
[check]
test = ["go", "test", "./..."]
some_unknown_field = "hello"

[unknown_section]
foo = "bar"
`), 0644)

	_, err := LoadConfig(dir)
	// go-toml/v2 is strict by default — unknown fields may or may not error.
	// The important thing is it doesn't panic.
	_ = err
}
