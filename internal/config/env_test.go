package config

import (
	"os"
	"testing"
)

func withTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "newo-config-test-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func withChdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
}

func TestLoadEnvDefaults(t *testing.T) {
	dir := withTempDir(t)
	withChdir(t, dir)

	t.Setenv("NEWO_API_KEY", "dummy-key")
	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if env.OutputRoot != defaultCustomersRoot {
		t.Fatalf("expected default output root %q, got %q", defaultCustomersRoot, env.OutputRoot)
	}
	if env.BaseURL != defaultBaseURL {
		t.Fatalf("expected default base url %q, got %q", defaultBaseURL, env.BaseURL)
	}
}

func TestLoadEnvTomlMerge(t *testing.T) {
	dir := withTempDir(t)
	withChdir(t, dir)

	toml := `
[defaults]
output_root = "custom_root"
slug_prefix = "prefix-"
base_url = "https://example.com"
default_customer = "ACME"

[[customers]]
idn = "ACME"
api_key = "toml-key"
project_id = "11111111-1111-1111-1111-111111111111"
`
	if err := os.WriteFile("newo.toml", []byte(toml), 0o644); err != nil {
		t.Fatalf("write toml: %v", err)
	}

	t.Setenv("NEWO_API_KEYS", `[]`) // ensure validation passes
	env, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if env.OutputRoot != "custom_root" {
		t.Fatalf("expected output_root merged, got %q", env.OutputRoot)
	}
	if env.SlugPrefix != "prefix-" {
		t.Fatalf("expected slug prefix merged, got %q", env.SlugPrefix)
	}
	if env.BaseURL != "https://example.com" {
		t.Fatalf("expected base url merged, got %q", env.BaseURL)
	}
	if env.DefaultCustomer != "ACME" {
		t.Fatalf("expected default customer ACME, got %q", env.DefaultCustomer)
	}
	if len(env.FileCustomers) != 1 || env.FileCustomers[0].APIKey != "toml-key" {
		t.Fatalf("expected customer from toml, got %#v", env.FileCustomers)
	}
}

func TestLoadEnvInvalidProjectID(t *testing.T) {
	dir := withTempDir(t)
	withChdir(t, dir)

	t.Setenv("NEWO_API_KEY", "dummy")
	t.Setenv("NEWO_PROJECT_ID", "not-a-uuid")

	if _, err := LoadEnv(); err == nil {
		t.Fatalf("expected error for invalid project id")
	}
}

func TestLooksLikeUUID(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"11111111-1111-1111-1111-111111111111", true},
		{"11111111111111111111111111111111", false},
		{"not-a-uuid", false},
	}
	for _, tc := range cases {
		if got := looksLikeUUID(tc.value); got != tc.want {
			t.Fatalf("looksLikeUUID(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
