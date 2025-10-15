package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/fsutil"
)

type tomlConfig struct {
	Defaults struct {
		OutputRoot *string `toml:"output_root"`
	} `toml:"defaults"`
}

// getOutputRoot is a shared helper for commands that need to know the output root
// but do not require full environment loading (e.g. lint, fmt).
func getOutputRoot() (string, error) {
	// 1. Check environment variable
	if root := strings.TrimSpace(os.Getenv("NEWO_OUTPUT_ROOT")); root != "" {
		return root, nil
	}

	// 2. Check newo.toml
	path := filepath.Join(".", config.DefaultTomlPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 3. Fallback to default
			return fsutil.DefaultCustomersDir, nil
		}
		return "", fmt.Errorf("read %s: %w", config.DefaultTomlPath, err)
	}

	var cfg tomlConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", config.DefaultTomlPath, err)
	}

	if cfg.Defaults.OutputRoot != nil {
		return strings.TrimSpace(*cfg.Defaults.OutputRoot), nil
	}

	// 3. Fallback to default
	return fsutil.DefaultCustomersDir, nil
}
