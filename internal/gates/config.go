package gates

import (
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Config holds optional gate overrides from repo-root gate.toml.
type Config struct {
	Check CheckConfig `toml:"check"`
}

// CheckConfig overrides auto-detected commands for a repo.
type CheckConfig struct {
	Test         []string     `toml:"test"`
	Truthsayer   []string     `toml:"truthsayer"`
	TruthsayerCI []string     `toml:"truthsayer_ci"`
	UBS          []string     `toml:"ubs"`
	UBSDiff      []string     `toml:"ubs_diff"`
	Lint         []LinterSpec `toml:"lint"`
}

// LinterSpec describes one configured or auto-detected linter.
type LinterSpec struct {
	Name string   `toml:"name"`
	Cmd  []string `toml:"cmd"`
}

// LoadConfig reads repo-root gate.toml if present.
func LoadConfig(dir string) (*Config, error) {
	path := filepath.Join(dir, "gate.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
