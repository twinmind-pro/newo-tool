package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeAPIKeyForToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/api-key/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("missing api key header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"abc","refresh_token":"ref","expires_in":5}`))
	}))
	t.Cleanup(srv.Close)

	tokens, err := ExchangeAPIKeyForToken(context.Background(), srv.URL, "test-key")
	if err != nil {
		t.Fatalf("ExchangeAPIKeyForToken: %v", err)
	}
	if tokens.AccessToken != "abc" || tokens.RefreshToken != "ref" {
		t.Fatalf("unexpected tokens: %#v", tokens)
	}
	if tokens.ExpiresInSeconds() == 0 {
		t.Fatalf("expected positive expiry")
	}
}

func TestRefreshAccessToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new","expires_at":0}`))
	}))
	t.Cleanup(srv.Close)

	tokens, err := RefreshAccessToken(context.Background(), srv.URL, "refresh")
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	if tokens.AccessToken != "new" {
		t.Fatalf("unexpected access token: %q", tokens.AccessToken)
	}
}

func TestExchangeAPIKeyForTokenHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	if _, err := ExchangeAPIKeyForToken(context.Background(), srv.URL, "key"); err == nil {
		t.Fatalf("expected error on non-2xx response")
	}
}
