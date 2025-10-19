package config

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/twinmind/newo-tool/internal/fsutil"
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

func TestTomlLoading(t *testing.T) {
	testCases := []struct {
		name          string
		tomlContent   string
		wantErr       string
		wantCustomers []FileCustomer
	}{
		{
			name: "success: multi-project and multi-customer",
			tomlContent: `
[[customers]]
  idn = "CUST1"
  api_key = "key1"
  type = "e2e"
  [[customers.projects]]
    idn = "projA"
    id = "uuid-A"
  [[customers.projects]]
    idn = "projB"
    id = "uuid-B"

[[customers]]
  idn = "CUST2"
  api_key = "key2"
  type = "integration"
  [[customers.projects]]
    idn = "projC"

[[customers]]
  idn = "CUST3"
  api_key = "key3"
  type = "e2e"
`, // No projects
			wantCustomers: []FileCustomer{
				{IDN: "CUST1", APIKey: "key1", Type: "e2e", Projects: []Project{{IDN: "projA", ID: "uuid-A"}, {IDN: "projB", ID: "uuid-B"}}},
				{IDN: "CUST2", APIKey: "key2", Type: "integration", Projects: []Project{{IDN: "projC"}}},
				{IDN: "CUST3", APIKey: "key3", Type: "e2e", Projects: nil},
			},
		},
		{
			name: "error: project idn collision",
			tomlContent: `
[[customers]]
  idn = "CUST1"
  api_key = "key1"
  type = "integration"
  [[customers.projects]]
    idn = "colliding-project"

[[customers]]
  idn = "CUST2"
  api_key = "key2"
  type = "integration"
  [[customers.projects]]
    idn = "colliding-project"
`,
			wantErr: "project IDN collision: project 'colliding-project' is defined for both customer 'CUST1' and customer 'CUST2' with type 'integration'",
		},
		{
			name: "success: no collision for non-integration types",
			tomlContent: `
[[customers]]
  idn = "CUST1"
  api_key = "key1"
  type = "e2e"
  [[customers.projects]]
    idn = "non-colliding-project"

[[customers]]
  idn = "CUST2"
  api_key = "key2"
  type = "e2e"
  [[customers.projects]]
    idn = "non-colliding-project"
`,
			wantCustomers: []FileCustomer{
				{IDN: "CUST1", APIKey: "key1", Type: "e2e", Projects: []Project{{IDN: "non-colliding-project"}}},
				{IDN: "CUST2", APIKey: "key2", Type: "e2e", Projects: []Project{{IDN: "non-colliding-project"}}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withTempDir(t)
			withChdir(t, dir)

			if err := os.WriteFile("newo.toml", []byte(tc.tomlContent), fsutil.FilePerm); err != nil {
				t.Fatalf("write toml: %v", err)
			}

			t.Setenv("NEWO_API_KEYS", `[]`) // ensure validation passes
			env, err := LoadEnv()

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("LoadEnv() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("LoadEnv() error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("LoadEnv() unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.wantCustomers, env.FileCustomers); diff != "" {
				t.Errorf("FileCustomers mismatch (-want +got):\n%s", diff)
			}
		})
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
