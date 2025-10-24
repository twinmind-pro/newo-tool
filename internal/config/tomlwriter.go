package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// FileCustomerWritable mirrors FileCustomer but is writable back to TOML.
type FileCustomerWritable struct {
	IDN      string    `toml:"idn"`
	Alias    string    `toml:"alias"`
	APIKey   string    `toml:"api_key"`
	Type     string    `toml:"type"`
	Projects []Project `toml:"projects"`
}

// TomlFile represents the structure of newo.toml.
type TomlFile struct {
	Defaults struct {
		OutputRoot         *string `toml:"output_root"`
		SlugPrefix         string  `toml:"slug_prefix"`
		IncludeHidden      bool    `toml:"include_hidden_attributes"`
		BaseURL            string  `toml:"base_url"`
		DefaultCustomerIDN string  `toml:"default_customer"`
		ProjectID          string  `toml:"project_id"`
		ProjectIDN         string  `toml:"project_idn"`
	} `toml:"defaults"`
	Customers []FileCustomerWritable `toml:"customers"`
	LLMs      []struct {
		Provider string `toml:"provider"`
		Model    string `toml:"model"`
		APIKey   string `toml:"api_key"`
	} `toml:"llms"`
}

// LoadToml loads newo.toml into a TomlFile structure.
func LoadToml(path string) (TomlFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TomlFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg TomlFile
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return TomlFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// SaveToml writes the provided config back to disk with stable ordering.
func SaveToml(path string, cfg TomlFile) error {
	ordered := normaliseToml(cfg)
	buf := bytes.Buffer{}
	if err := toml.NewEncoder(&buf).Encode(ordered); err != nil {
		return fmt.Errorf("encode toml: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func normaliseToml(cfg TomlFile) TomlFile {
	if len(cfg.Customers) > 1 {
		sort.Slice(cfg.Customers, func(i, j int) bool {
			return cfg.Customers[i].IDN < cfg.Customers[j].IDN
		})
	}
	for idx := range cfg.Customers {
		projects := cfg.Customers[idx].Projects
		if len(projects) > 1 {
			sort.Slice(projects, func(i, j int) bool {
				return projects[i].IDN < projects[j].IDN
			})
			cfg.Customers[idx].Projects = projects
		}
	}
	return cfg
}

var errCustomerNotFound = errors.New("customer not found")

// AddProjectToToml ensures the given project is listed under the target customer.
func AddProjectToToml(path, customerIDN, projectIDN, projectID string) error {
	cfg, err := LoadToml(path)
	if err != nil {
		return err
	}

	idx := -1
	for i := range cfg.Customers {
		if strings.EqualFold(cfg.Customers[i].IDN, customerIDN) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("%w: %s", errCustomerNotFound, customerIDN)
	}

	projects := cfg.Customers[idx].Projects
	updated := false
	for i := range projects {
		if strings.EqualFold(projects[i].IDN, projectIDN) {
			cfg.Customers[idx].Projects[i].ID = projectID
			updated = true
			break
		}
	}

	if !updated {
		cfg.Customers[idx].Projects = append(cfg.Customers[idx].Projects, Project{IDN: projectIDN, ID: projectID})
	}

	return SaveToml(path, cfg)
}
