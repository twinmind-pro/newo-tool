package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/twinmind/newo-tool/internal/fsutil"
)

// Tokens represents the stored authentication tokens for a customer.
type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

const (
	tokenExpiryDrift = 60 * time.Second
)

// Load returns cached tokens for the customer, if present.
func Load(customerIDN string) (Tokens, bool, error) {
	path := filepath.Join(fsutil.CustomerStateDir(customerIDN), "tokens.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Tokens{}, false, nil
		}
		return Tokens{}, false, fmt.Errorf("read tokens: %w", err)
	}

	var tokens Tokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return Tokens{}, false, fmt.Errorf("decode tokens: %w", err)
	}

	return tokens, true, nil
}

// Save persists tokens for the customer.
func Save(customerIDN string, tokens Tokens) error {
	if tokens.AccessToken == "" {
		return errors.New("access token is required")
	}
	if tokens.ExpiresAt.IsZero() {
		return errors.New("expiry timestamp is required")
	}

	path := filepath.Join(fsutil.CustomerStateDir(customerIDN), "tokens.json")
	if err := fsutil.EnsureParentDir(path); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tokens: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write tokens: %w", err)
	}
	return nil
}

// IsExpired reports whether the token is expired or about to expire.
func (t Tokens) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return true
	}
	return time.Until(t.ExpiresAt) <= tokenExpiryDrift
}

// CanRefresh reports whether a refresh token is available.
func (t Tokens) CanRefresh() bool {
	return t.RefreshToken != ""
}
