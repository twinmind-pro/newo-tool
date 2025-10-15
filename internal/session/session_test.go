package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/twinmind/newo-tool/internal/auth"
	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/state"
)

func TestNewSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v1/auth/api-key/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"access_token":"abc","refresh_token":"ref","expires_in":%d}`, int(time.Hour.Seconds()))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/api/v1/customer/profile") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cust_123","idn":"ACME"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	t.Run("handles save token error", func(t *testing.T) {
		// Setup
		tmp := t.TempDir()
		wd, _ := os.Getwd()
		_ = os.Chdir(tmp)
		t.Cleanup(func() { _ = os.Chdir(wd) })
		if err := os.MkdirAll(fsutil.StateDirName, fsutil.DirPerm); err != nil {
			t.Fatal(err)
		}

		originalSave := auth.Save
		auth.Save = func(customerIDN string, tokens auth.Tokens) error {
			return errors.New("disk is full")
		}
		t.Cleanup(func() { auth.Save = originalSave })

		env := config.Env{BaseURL: server.URL}
		entry := customer.Entry{APIKey: "test-key"}
		registry := state.NewAPIKeyRegistry()

		// Execute
		_, err := New(context.Background(), env, entry, registry)

		// Assert
		if err == nil {
			t.Fatal("expected an error when saving tokens fails, but got nil")
		}
		if !strings.Contains(err.Error(), "disk is full") {
			t.Fatalf("expected error to contain 'disk is full', but got: %v", err)
		}
	})

	t.Run("creates session successfully", func(t *testing.T) {
		// Setup
		tmp := t.TempDir()
		wd, _ := os.Getwd()
		_ = os.Chdir(tmp)
		t.Cleanup(func() { _ = os.Chdir(wd) })
		if err := os.MkdirAll(fsutil.StateDirName, fsutil.DirPerm); err != nil {
			t.Fatal(err)
		}

		var savedCustomerIDN string
		originalSave := auth.Save
		auth.Save = func(customerIDN string, tokens auth.Tokens) error {
			savedCustomerIDN = customerIDN
			return originalSave(customerIDN, tokens)
		}
		t.Cleanup(func() { auth.Save = originalSave })

		env := config.Env{BaseURL: server.URL}
		entry := customer.Entry{APIKey: "test-key"}
		registry := state.NewAPIKeyRegistry()

		// Execute
		s, err := New(context.Background(), env, entry, registry)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if s == nil {
			t.Fatal("expected session to be non-nil")
		}
		if s.IDN != "ACME" {
			t.Errorf("expected session IDN to be 'ACME', got %q", s.IDN)
		}
		if savedCustomerIDN != "acme" {
			t.Errorf("expected auth.Save to be called with 'acme', got %q", savedCustomerIDN)
		}
	})
}
