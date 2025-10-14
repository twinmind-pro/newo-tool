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
