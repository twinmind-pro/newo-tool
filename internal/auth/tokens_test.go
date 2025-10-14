package auth

import (
	"os"
	"testing"
	"time"

	"github.com/twinmind/newo-tool/internal/fsutil"
)

func TestTokensSaveLoad(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	customer := "TEST"
	if err := fsutil.EnsureWorkspace(customer); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	tokens := Tokens{AccessToken: "abc", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Minute)}
	if err := Save(customer, tokens); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, ok, err := Load(customer)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok {
		t.Fatalf("expected tokens loaded")
	}
	if loaded.AccessToken != tokens.AccessToken || loaded.RefreshToken != tokens.RefreshToken {
		t.Fatalf("loaded mismatch: %#v", loaded)
	}
}

func TestIsExpired(t *testing.T) {
	t.Parallel()

	zero := Tokens{}
	if !zero.IsExpired() {
		t.Fatalf("zero tokens should be expired")
	}
	valid := Tokens{ExpiresAt: time.Now().Add(2 * time.Minute)}
	if valid.IsExpired() {
		t.Fatalf("token with sufficient future expiry should not be expired")
	}
	soon := Tokens{ExpiresAt: time.Now().Add(5 * time.Second)}
	if !soon.IsExpired() {
		t.Fatalf("token expiring soon should be treated as expired")
	}
	if (Tokens{}).CanRefresh() {
		t.Fatalf("empty tokens should not be refreshable")
	}
	if !(Tokens{RefreshToken: "ref"}).CanRefresh() {
		t.Fatalf("refresh token should be detected")
	}
}
