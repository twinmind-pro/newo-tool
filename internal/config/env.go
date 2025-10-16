package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Env holds validated environment variables required by the CLI.
type Env struct {
	BaseURL             string
	ProjectID           string
	APIKey              string
	APIKeysJSON         string
	AccessToken         string
	RefreshToken        string
	RefreshURL          string
	DefaultCustomer     string
	FileCustomers       []FileCustomer
	FileDefaultCustomer string
	OutputRoot          string
	SlugPrefix          string
	FileLLMs            []LLMConfig // Added
}

// FileCustomer describes a customer defined in newo.toml.
type FileCustomer struct {
	IDN       string
	APIKey    string
	ProjectID string
}

// LLMConfig describes an LLM configuration defined in newo.toml.
type LLMConfig struct {
	Provider string
	Model    string
	APIKey   string
}

// LoadEnv reads environment variables, applies defaults, merges newo.toml, and validates values.
func LoadEnv() (Env, error) {
	baseURL := strings.TrimSpace(os.Getenv("NEWO_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if err := validateURL(baseURL, "NEWO_BASE_URL"); err != nil {
		return Env{}, err
	}

	env := Env{
		BaseURL:         baseURL,
		ProjectID:       strings.TrimSpace(os.Getenv("NEWO_PROJECT_ID")),
		APIKey:          strings.TrimSpace(os.Getenv("NEWO_API_KEY")),
		APIKeysJSON:     strings.TrimSpace(os.Getenv("NEWO_API_KEYS")),
		AccessToken:     strings.TrimSpace(os.Getenv("NEWO_ACCESS_TOKEN")),
		RefreshToken:    strings.TrimSpace(os.Getenv("NEWO_REFRESH_TOKEN")),
		RefreshURL:      strings.TrimSpace(os.Getenv("NEWO_REFRESH_URL")),
		DefaultCustomer: strings.TrimSpace(os.Getenv("NEWO_DEFAULT_CUSTOMER")),
		OutputRoot:      strings.TrimSpace(os.Getenv("NEWO_OUTPUT_ROOT")),
		SlugPrefix:      strings.TrimSpace(os.Getenv("NEWO_SLUG_PREFIX")),
	}

	var isOutputRootSetInToml bool
	if err := mergeTomlConfig(&env, &isOutputRootSetInToml); err != nil {
		return Env{}, err
	}

	if env.OutputRoot == "" && !isOutputRootSetInToml {
		env.OutputRoot = defaultCustomersRoot
	}

	if env.ProjectID != "" && !looksLikeUUID(env.ProjectID) {
		return Env{}, fmt.Errorf("NEWO_PROJECT_ID must be a valid UUID, got %q", env.ProjectID)
	}

	if env.RefreshURL != "" {
		if err := validateURL(env.RefreshURL, "NEWO_REFRESH_URL"); err != nil {
			return Env{}, err
		}
	}
	return env, nil
}

func validateURL(raw, name string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s must be a valid absolute URL, got %q", name, raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme, got %q", name, raw)
	}
	return nil
}

func looksLikeUUID(value string) bool {
	const uuidLength = 36
	if len(value) != uuidLength {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
	}
	return true
}

const (
	defaultBaseURL       = "https://app.newo.ai"
	defaultCustomersRoot = "newo_customers"
	DefaultTomlPath      = "newo.toml"
)

type tomlConfig struct {
	Defaults struct {
		OutputRoot         *string `toml:"output_root"`
		SlugPrefix         string  `toml:"slug_prefix"`
		IncludeHidden      bool    `toml:"include_hidden_attributes"`
		BaseURL            string  `toml:"base_url"`
		DefaultCustomerIDN string  `toml:"default_customer"`
		ProjectID          string  `toml:"project_id"`
	} `toml:"defaults"`
	Customers []struct {
		IDN       string `toml:"idn"`
		APIKey    string `toml:"api_key"`
		ProjectID string `toml:"project_id"`
	} `toml:"customers"`
	LLMs []struct {
		Provider string `toml:"provider"`
		Model    string `toml:"model"`
		APIKey   string `toml:"api_key"`
	} `toml:"llms"` // Added
}

func mergeTomlConfig(env *Env, isOutputRootSetInToml *bool) error {
	path := filepath.Join(".", DefaultTomlPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", DefaultTomlPath, err)
	}

	var cfg tomlConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", DefaultTomlPath, err)
	}

	if base := strings.TrimSpace(cfg.Defaults.BaseURL); base != "" && env.BaseURL == defaultBaseURL {
		env.BaseURL = base
	}
	if project := strings.TrimSpace(cfg.Defaults.ProjectID); project != "" && env.ProjectID == "" {
		env.ProjectID = project
	}
	if defCustomer := strings.TrimSpace(cfg.Defaults.DefaultCustomerIDN); defCustomer != "" && env.DefaultCustomer == "" {
		env.DefaultCustomer = defCustomer
		env.FileDefaultCustomer = defCustomer
	}
	if cfg.Defaults.OutputRoot != nil {
		*isOutputRootSetInToml = true
		env.OutputRoot = strings.TrimSpace(*cfg.Defaults.OutputRoot)
	}
	if slug := strings.TrimSpace(cfg.Defaults.SlugPrefix); slug != "" && env.SlugPrefix == "" {
		env.SlugPrefix = slug
	}

	for _, c := range cfg.Customers {
		apiKey := strings.TrimSpace(c.APIKey)
		if apiKey == "" {
			continue
		}
		env.FileCustomers = append(env.FileCustomers, FileCustomer{
			IDN:       strings.TrimSpace(c.IDN),
			APIKey:    apiKey,
			ProjectID: strings.TrimSpace(c.ProjectID),
		})
	}

	// Populate LLM configurations
	for _, l := range cfg.LLMs {
		apiKey := strings.TrimSpace(l.APIKey)
		if apiKey == "" {
			continue
		}
		env.FileLLMs = append(env.FileLLMs, LLMConfig{
			Provider: strings.TrimSpace(l.Provider),
			Model:    strings.TrimSpace(l.Model),
			APIKey:   apiKey,
		})
	}

	return nil
}
