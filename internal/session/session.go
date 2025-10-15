package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/twinmind/newo-tool/internal/auth"
	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/state"
)

// Session represents an authenticated customer session for interacting with the API.
type Session struct {
	IDN             string
	ProjectID       string
	Client          *platform.Client
	Tokens          auth.Tokens
	Profile         platform.CustomerProfile
	RegistryUpdated bool
}

// New creates a new authenticated session for a given customer entry.
func New(ctx context.Context, env config.Env, entry customer.Entry, registry *state.APIKeyRegistry) (*Session, error) {
	knownIDN := strings.TrimSpace(entry.HintIDN)
	if knownIDN == "" {
		if idn, ok := registry.Lookup(entry.APIKey); ok {
			knownIDN = idn
		}
	}

	var tokens auth.Tokens
	haveTokens := false
	if knownIDN != "" {
		cached, ok, err := auth.Load(knownIDN)
		if err != nil {
			return nil, err
		}
		if ok {
			tokens = cached
			haveTokens = true
		}
	}

	refreshed := false
	if haveTokens && tokens.IsExpired() && tokens.CanRefresh() && env.RefreshURL != "" {
		// Verbose logging should be handled by the caller
		resp, err := platform.RefreshAccessToken(ctx, env.RefreshURL, tokens.RefreshToken)
		if err != nil {
			// Log warning in caller
		} else {
			fresh, convErr := auth.FromResponse(resp)
			if convErr != nil {
				return nil, convErr
			}
			tokens = fresh
			refreshed = true
		}
	}

	if !haveTokens || tokens.IsExpired() {
		// Verbose logging in caller
		resp, err := platform.ExchangeAPIKeyForToken(ctx, env.BaseURL, entry.APIKey)
		if err != nil {
			return nil, fmt.Errorf("exchange api key: %w", err)
		}
		fresh, convErr := auth.FromResponse(resp)
		if convErr != nil {
			return nil, convErr
		}
		tokens = fresh
		refreshed = true
	}

	client, err := platform.NewClient(env.BaseURL, tokens.AccessToken)
	if err != nil {
		return nil, err
	}

	profile, err := client.GetCustomerProfile(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch customer profile: %w", err)
	}
	if profile.IDN == "" {
		return nil, fmt.Errorf("customer profile response missing idn")
	}

	registryUpdated := false
	if knownIDN == "" || !strings.EqualFold(knownIDN, profile.IDN) {
		registry.Register(entry.APIKey, profile.IDN)
		registryUpdated = true
	}

	if refreshed || knownIDN == "" || !strings.EqualFold(knownIDN, profile.IDN) {
		if err := auth.Save(strings.ToLower(profile.IDN), tokens); err != nil {
			return nil, fmt.Errorf("persist tokens: %w", err)
		}
	}

	return &Session{
		IDN:             profile.IDN,
		ProjectID:       entry.ProjectID,
		Client:          client,
		Tokens:          tokens,
		Profile:         profile,
		RegistryUpdated: registryUpdated,
	}, nil
}
