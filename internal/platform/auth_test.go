package platform

import (
	"context"
	"net/http"
	"testing"

	"github.com/twinmind/newo-tool/internal/testutil/httpmock"
)

func TestExchangeAPIKeyForToken(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/api-key/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("missing api key header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"abc","refresh_token":"ref","expires_in":5}`))
	})
	client, _ := httpmock.New(handler)
	t.Cleanup(SetHTTPClientForTesting(client))

	tokens, err := ExchangeAPIKeyForToken(context.Background(), httpmock.BaseURL, "test-key")
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new","expires_at":0}`))
	})
	client, _ := httpmock.New(handler)
	t.Cleanup(SetHTTPClientForTesting(client))

	tokens, err := RefreshAccessToken(context.Background(), httpmock.BaseURL+"/api/v1/auth/refresh", "refresh")
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	if tokens.AccessToken != "new" {
		t.Fatalf("unexpected access token: %q", tokens.AccessToken)
	}
}

func TestExchangeAPIKeyForTokenHTTPError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusBadRequest)
	})
	client, _ := httpmock.New(handler)
	t.Cleanup(SetHTTPClientForTesting(client))

	if _, err := ExchangeAPIKeyForToken(context.Background(), httpmock.BaseURL, "key"); err == nil {
		t.Fatalf("expected error on non-2xx response")
	}
}
