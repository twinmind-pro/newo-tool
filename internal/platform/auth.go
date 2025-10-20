package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

// TokenResponse represents authentication tokens returned by the API key exchange endpoint.
type TokenResponse struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresInRaw json.RawMessage `json:"expires_in"`
	Token        string          `json:"token"` // fallback field name used by some responses
	ExpiresAt    int64           `json:"expires_at"`
}

var httpClient = http.DefaultClient

// SetHTTPClientForTesting overrides the HTTP client used by auth helpers. The caller must invoke the returned
// cleanup function to restore the previous client once the test completes.
func SetHTTPClientForTesting(client *http.Client) func() {
	prev := httpClient
	if client == nil {
		client = http.DefaultClient
	}
	httpClient = client
	return func() {
		httpClient = prev
	}
}

// ExchangeAPIKeyForToken exchanges an API key for tokens.
func ExchangeAPIKeyForToken(ctx context.Context, baseURL, apiKey string) (TokenResponse, error) {
	if apiKey == "" {
		return TokenResponse{}, fmt.Errorf("api key is required")
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("parse base url: %w", err)
	}
	u.Path = path.Join(u.Path, "/api/v1/auth/api-key/token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("exchange api key: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("exchange api key: status %d", resp.StatusCode)
	}

	var tokens TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return TokenResponse{}, fmt.Errorf("decode token response: %w", err)
	}

	// Some responses use "token" instead of "access_token".
	if tokens.AccessToken == "" && tokens.Token != "" {
		tokens.AccessToken = tokens.Token
	}

	if tokens.AccessToken == "" {
		return TokenResponse{}, fmt.Errorf("token response missing access token")
	}

	return tokens, nil
}

// RefreshAccessToken exchanges a refresh token for new access credentials.
func RefreshAccessToken(ctx context.Context, refreshURL, refreshToken string) (TokenResponse, error) {
	if refreshURL == "" {
		return TokenResponse{}, fmt.Errorf("refresh url is required")
	}
	if refreshToken == "" {
		return TokenResponse{}, fmt.Errorf("refresh token is required")
	}

	payload, err := json.Marshal(map[string]string{"refresh_token": refreshToken})
	if err != nil {
		return TokenResponse{}, fmt.Errorf("encode refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(payload))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("refresh access token: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("refresh access token: status %d", resp.StatusCode)
	}

	var tokens TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return TokenResponse{}, fmt.Errorf("decode refresh response: %w", err)
	}

	if tokens.AccessToken == "" && tokens.Token != "" {
		tokens.AccessToken = tokens.Token
	}

	if tokens.AccessToken == "" {
		return TokenResponse{}, fmt.Errorf("refresh response missing access token")
	}

	return tokens, nil
}

// ExpiresInSeconds normalises the expires_in/ExpiresAt values into seconds.
func (t TokenResponse) ExpiresInSeconds() int {
	if len(t.ExpiresInRaw) > 0 {
		// expires_in may come as number or quoted string
		if t.ExpiresInRaw[0] == '"' {
			var s string
			if err := json.Unmarshal(t.ExpiresInRaw, &s); err == nil {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					return clampExpires(int(v))
				}
			}
		} else {
			var v float64
			if err := json.Unmarshal(t.ExpiresInRaw, &v); err == nil {
				return clampExpires(int(v))
			}
		}
	}

	if t.ExpiresAt != 0 {
		seconds := int(time.Until(time.Unix(t.ExpiresAt, 0)).Seconds())
		return clampExpires(seconds)
	}

	return 0
}

func clampExpires(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
