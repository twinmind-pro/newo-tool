package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/util"
)

// APIKeyRegistry keeps a mapping between API key fingerprints and customer IDNs.
type APIKeyRegistry struct {
	entries map[string]string
}

// NewAPIKeyRegistry creates a new, empty registry.
func NewAPIKeyRegistry() *APIKeyRegistry {
	return &APIKeyRegistry{entries: map[string]string{}}
}

// LoadAPIKeyRegistry returns the persisted API key registry.
func LoadAPIKeyRegistry() (*APIKeyRegistry, error) {
	path := fsutil.APIKeyRegistryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewAPIKeyRegistry(), nil
		}
		return nil, fmt.Errorf("read api key registry: %w", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode api key registry: %w", err)
	}
	return &APIKeyRegistry{entries: payload}, nil
}

// Save persists the registry.
func (r *APIKeyRegistry) Save() error {
	if r.entries == nil {
		r.entries = map[string]string{}
	}
	path := fsutil.APIKeyRegistryPath()
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode api key registry: %w", err)
	}
	if err := os.WriteFile(path, data, fsutil.FilePerm); err != nil {
		return fmt.Errorf("write api key registry: %w", err)
	}
	return nil
}

// Lookup returns the customer IDN for a given API key, if known.
func (r *APIKeyRegistry) Lookup(apiKey string) (string, bool) {
	if r.entries == nil {
		return "", false
	}
	idn, ok := r.entries[hashKey(apiKey)]
	return idn, ok
}

// Register associates an API key with a customer IDN.
func (r *APIKeyRegistry) Register(apiKey, customerIDN string) {
	if r.entries == nil {
		r.entries = map[string]string{}
	}
	r.entries[hashKey(apiKey)] = customerIDN
}

func hashKey(apiKey string) string {
	return util.SHA256String(apiKey)
}
