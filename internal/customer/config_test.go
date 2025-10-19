package customer

import (
	"testing"

	"github.com/twinmind/newo-tool/internal/config"
)

func TestFromEnvParsesAPIKeysJSON(t *testing.T) {
	env := config.Env{
		APIKeysJSON: `[{"key":"k1","project_id":"p1","idn":"ACME"},"k2"]`,
	}
	cfg, err := FromEnv(env)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if len(cfg.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cfg.Entries))
	}
	if cfg.Entries[0].APIKey != "k1" || cfg.Entries[1].APIKey != "k2" {
		t.Fatalf("unexpected entries: %#v", cfg.Entries)
	}
	if cfg.DefaultCustomer != "" {
		t.Fatalf("unexpected default customer: %q", cfg.DefaultCustomer)
	}
}

func TestFromEnvRequiresConfig(t *testing.T) {
	if _, err := FromEnv(config.Env{}); err == nil {
		t.Fatalf("expected error when no configuration provided")
	}
}

func TestFromEnvPrecedence(t *testing.T) {
	t.Run("when file has customers, env is ignored", func(t *testing.T) {
		env := config.Env{
			// This key from env should be ignored
			APIKey: "env-api-key",

			// This customer from file should take precedence
			FileCustomers: []config.FileCustomer{
				{
					IDN:    "file-customer",
					APIKey: "file-api-key",
					Type:   "e2e",
					Projects: []config.Project{
						{IDN: "file-project"},
					},
				},
			},
		}

		cfg, err := FromEnv(env)
		if err != nil {
			t.Fatalf("FromEnv() unexpected error: %v", err)
		}

		if len(cfg.Entries) != 1 {
			t.Fatalf("expected 1 entry from file, got %d", len(cfg.Entries))
		}

		if cfg.Entries[0].APIKey != "file-api-key" {
			t.Errorf("expected entry to be from file customer, but got API key %q", cfg.Entries[0].APIKey)
		}
	})

	t.Run("when file has no customers, env is used", func(t *testing.T) {
		env := config.Env{
			// This key from env should be used
			APIKey: "env-api-key",

			// No customers from file
			FileCustomers: []config.FileCustomer{},
		}

		cfg, err := FromEnv(env)
		if err != nil {
			t.Fatalf("FromEnv() unexpected error: %v", err)
		}

		if len(cfg.Entries) != 1 {
			t.Fatalf("expected 1 entry from env, got %d", len(cfg.Entries))
		}

		if cfg.Entries[0].APIKey != "env-api-key" {
			t.Errorf("expected entry to be from env, but got API key %q", cfg.Entries[0].APIKey)
		}
	})
}