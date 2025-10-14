package customer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/twinmind/newo-tool/internal/config"
)

// Entry represents a single customer bootstrap configuration.
type Entry struct {
	APIKey    string
	ProjectID string
	HintIDN   string
}

// Configuration aggregates customer entries and default selection.
type Configuration struct {
	Entries         []Entry
	DefaultCustomer string
}

// FromEnv parses customer configuration from environment variables.
func FromEnv(env config.Env) (Configuration, error) {
	var entries []Entry

	if env.APIKeysJSON != "" {
		parsed, err := parseAPIKeysJSON(env.APIKeysJSON)
		if err != nil {
			return Configuration{}, err
		}
		entries = append(entries, parsed...)
	}

	if env.APIKey != "" {
		entries = append(entries, Entry{
			APIKey:    env.APIKey,
			ProjectID: env.ProjectID,
		})
	}

	for _, fileCustomer := range env.FileCustomers {
		entries = append(entries, Entry{
			APIKey:    fileCustomer.APIKey,
			ProjectID: fileCustomer.ProjectID,
			HintIDN:   fileCustomer.IDN,
		})
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
			Key       string `json:"key"`
			ProjectID string `json:"project_id"`
			IDN       string `json:"idn"`
		}
		if err := json.Unmarshal(item, &obj); err != nil {
			return nil, fmt.Errorf("NEWO_API_KEYS[%d]: %w", idx, err)
		}

		obj.Key = strings.TrimSpace(obj.Key)
		if obj.Key == "" {
			return nil, fmt.Errorf("NEWO_API_KEYS[%d] missing key", idx)
		}

		entries = append(entries, Entry{
			APIKey:    obj.Key,
			ProjectID: strings.TrimSpace(obj.ProjectID),
			HintIDN:   strings.TrimSpace(obj.IDN),
		})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("NEWO_API_KEYS is empty")
	}
	return entries, nil
}
