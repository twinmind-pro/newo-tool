package auth

import (
	"fmt"
	"time"

	"github.com/twinmind/newo-tool/internal/platform"
)

// FromResponse converts a platform token response into Tokens.
func FromResponse(resp platform.TokenResponse) (Tokens, error) {
	if resp.AccessToken == "" {
		return Tokens{}, fmt.Errorf("token response missing access token")
	}
	expires := time.Now().Add(defaultTokenTTL)
	if seconds := resp.ExpiresInSeconds(); seconds > 0 {
		expires = time.Now().Add(time.Duration(seconds) * time.Second)
	}
	return Tokens{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    expires,
	}, nil
}

const defaultTokenTTL = 10 * time.Minute
