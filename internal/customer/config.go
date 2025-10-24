package customer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/twinmind/newo-tool/internal/config"
)

// Entry represents a single customer bootstrap configuration.
type Entry struct {
	APIKey     string
	ProjectID  string
	ProjectIDN string // Added to hold per-customer project IDN
	HintIDN    string
	Alias      string
	Type       string // Added to hold customer type
}

// Configuration aggregates customer entries and default selection.
type Configuration struct {
	Entries         []Entry
	DefaultCustomer string
}

// FindCustomer resolves a customer by IDN or alias.
func (cfg Configuration) FindCustomer(id string) (*Entry, error) {
	token := strings.TrimSpace(id)
	if token == "" {
		return nil, fmt.Errorf("customer identifier is required")
	}
	for idx := range cfg.Entries {
		entry := &cfg.Entries[idx]
		if strings.EqualFold(entry.HintIDN, token) || (entry.Alias != "" && strings.EqualFold(entry.Alias, token)) {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("customer %s not configured", token)
}

// FromEnv parses customer configuration from environment variables.
func FromEnv(env config.Env) (Configuration, error) {
	var entries []Entry

	// First, prioritize customers from the TOML file.
	if len(env.FileCustomers) > 0 {
		for _, fileCustomer := range env.FileCustomers {
			alias := strings.TrimSpace(fileCustomer.Alias)
			entry := Entry{
				APIKey:  fileCustomer.APIKey,
				HintIDN: fileCustomer.IDN,
				Alias:   alias,
				Type:    fileCustomer.Type,
			}
			if len(fileCustomer.Projects) == 0 {
				entries = append(entries, entry)
				continue
			}
			for _, p := range fileCustomer.Projects {
				sized := entry
				sized.ProjectID = p.ID
				sized.ProjectIDN = p.IDN
				entries = append(entries, sized)
			}
		}
	} else {
		// Only if no customers are in the file, fall back to environment variables.
		if env.APIKeysJSON != "" {
			parsed, err := parseAPIKeysJSON(env.APIKeysJSON)
			if err != nil {
				return Configuration{}, err
			}
			entries = append(entries, parsed...)
		}

		if env.APIKey != "" {
			entries = append(entries, Entry{
				APIKey:     env.APIKey,
				ProjectID:  env.ProjectID,
				ProjectIDN: env.ProjectIDN, // Also respect global project_idn
			})
		}
	}

	if len(entries) == 0 {
		if env.AccessToken != "" {
			return Configuration{}, fmt.Errorf("NEWO_ACCESS_TOKEN requires NEWO_API_KEY to refresh automatically")
		}
		return Configuration{}, fmt.Errorf("no customer configuration found; set NEWO_API_KEY, NEWO_API_KEYS, or configure newo.toml")
	}

	defaultCustomer := env.DefaultCustomer
	if defaultCustomer == "" {
		defaultCustomer = env.FileDefaultCustomer
	}

	return Configuration{
		Entries:         entries,
		DefaultCustomer: defaultCustomer,
	}, nil
}

func parseAPIKeysJSON(payload string) ([]Entry, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return nil, fmt.Errorf("parse NEWO_API_KEYS: %w", err)
	}

	entries := make([]Entry, 0, len(raw))
	for idx, item := range raw {
		// String form representing just the API key.
		var asString string
		if err := json.Unmarshal(item, &asString); err == nil {
			asString = strings.TrimSpace(asString)
			if asString == "" {
				return nil, fmt.Errorf("NEWO_API_KEYS[%d] is empty", idx)
			}
			entries = append(entries, Entry{APIKey: asString})
			continue
		}

		// Object form with optional metadata.
		var obj struct {
			Key        string `json:"key"`
			ProjectID  string `json:"project_id"`
			ProjectIDN string `json:"project_idn"`
			IDN        string `json:"idn"`
		}
		if err := json.Unmarshal(item, &obj); err != nil {
			return nil, fmt.Errorf("NEWO_API_KEYS[%d]: %w", idx, err)
		}

		obj.Key = strings.TrimSpace(obj.Key)
		if obj.Key == "" {
			return nil, fmt.Errorf("NEWO_API_KEYS[%d] missing key", idx)
		}

		entries = append(entries, Entry{
			APIKey:     obj.Key,
			ProjectID:  strings.TrimSpace(obj.ProjectID),
			ProjectIDN: strings.TrimSpace(obj.ProjectIDN),
			HintIDN:    strings.TrimSpace(obj.IDN),
		})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("NEWO_API_KEYS is empty")
	}
	return entries, nil
}
