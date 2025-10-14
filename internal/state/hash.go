package state

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/twinmind/newo-tool/internal/fsutil"
)

// HashStore maps relative file paths to their SHA-256 digest.
type HashStore map[string]string

// LoadHashes returns hashes stored for the customer, or an empty map if none exist.
func LoadHashes(customerIDN string) (HashStore, error) {
	path := fsutil.HashesPath(customerIDN)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return HashStore{}, nil
		}
		return nil, fmt.Errorf("read hashes: %w", err)
	}

	var hashes HashStore
	if err := json.Unmarshal(data, &hashes); err != nil {
		return nil, fmt.Errorf("decode hashes: %w", err)
	}
	return hashes, nil
}

// SaveHashes persists the given hash store.
func SaveHashes(customerIDN string, hashes HashStore) error {
	path := fsutil.HashesPath(customerIDN)
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}

	data, err := json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		return fmt.Errorf("encode hashes: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write hashes: %w", err)
	}
	return nil
}
