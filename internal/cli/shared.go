package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/state"
)

type tomlConfig struct {
	Defaults struct {
		OutputRoot *string `toml:"output_root"`
	} `toml:"defaults"`
}

// getOutputRoot returns the configured output root for customer data.
func getOutputRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("NEWO_OUTPUT_ROOT")); root != "" {
		return root, nil
	}

	path := filepath.Join(".", config.DefaultTomlPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
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

	return fsutil.DefaultCustomersDir, nil
}

// resolveCustomerDirectories returns directories to scan for the given customer filter.
func resolveCustomerDirectories(outputRoot, customer string) ([]string, string, bool, error) {
	customer = strings.TrimSpace(customer)
	if customer == "" {
		return []string{outputRoot}, "", false, nil
	}

	definition, err := loadCustomerDefinition(customer)
	if err != nil {
		return nil, "", false, err
	}
	if definition == nil {
		return nil, "", false, fmt.Errorf("customer %s not configured", customer)
	}

	dirs, hasProjects, err := customerProjectDirectories(outputRoot, definition)
	if err != nil {
		return nil, "", false, err
	}
	if !hasProjects {
		return nil, definition.IDN, true, nil
	}
	return dirs, definition.IDN, false, nil
}

type customerDefinition struct {
	IDN   string
	Alias string
	Type  string
}

func loadCustomerDefinition(token string) (*customerDefinition, error) {
	path := filepath.Join(".", config.DefaultTomlPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", config.DefaultTomlPath, err)
	}

	var cfg config.TomlConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", config.DefaultTomlPath, err)
	}

	token = strings.TrimSpace(token)
	lowerToken := strings.ToLower(token)

	type aliasRecord struct {
		idn   string
		alias string
		ctype string
	}
	aliasIndex := make(map[string]aliasRecord)

	for _, c := range cfg.Customers {
		cleanIDN := strings.TrimSpace(c.IDN)
		cleanAlias := strings.TrimSpace(c.Alias)
		if cleanAlias != "" {
			aliasIndex[strings.ToLower(cleanAlias)] = aliasRecord{idn: cleanIDN, alias: cleanAlias, ctype: strings.TrimSpace(c.Type)}
		}
		if strings.EqualFold(cleanIDN, token) {
			return &customerDefinition{IDN: cleanIDN, Alias: cleanAlias, Type: strings.TrimSpace(c.Type)}, nil
		}
	}

	if candidate, ok := aliasIndex[lowerToken]; ok {
		return &customerDefinition{IDN: candidate.idn, Alias: candidate.alias, Type: candidate.ctype}, nil
	}

	return nil, nil
}

func customerProjectDirectories(outputRoot string, definition *customerDefinition) ([]string, bool, error) {
	projectMap, err := state.LoadProjectMap(definition.IDN)
	if err != nil {
		return nil, false, err
	}
	if len(projectMap.Projects) == 0 {
		return nil, false, nil
	}

	unique := make(map[string]struct{}, len(projectMap.Projects))
	for projectIDN, data := range projectMap.Projects {
		slug := projectSlugFromState(projectIDN, data)
		dir := fsutil.ExportProjectDir(outputRoot, definition.Type, definition.IDN, slug)
		unique[dir] = struct{}{}
	}

	dirs := make([]string, 0, len(unique))
	for dir := range unique {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs, true, nil
}

func projectSlugFromState(projectIDN string, data state.ProjectData) string {
	slug := strings.TrimSpace(data.Path)
	if slug != "" {
		return slug
	}
	base := strings.TrimSpace(projectIDN)
	if base == "" {
		base = "project"
	}
	return strings.ToLower(base)
}
