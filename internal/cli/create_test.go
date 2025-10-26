package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/create"
	"github.com/twinmind/newo-tool/internal/customer"
	"github.com/twinmind/newo-tool/internal/deploy"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/session"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

type stubCreateService struct {
	lastReq create.Request
	result  deploy.DeployResult
	err     error
}

func (s *stubCreateService) Create(ctx context.Context, req create.Request) (deploy.DeployResult, error) {
	s.lastReq = req
	return s.result, s.err
}

func TestParseCreateArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantName     string
		wantCustomer string
		wantErr      bool
	}{
		{
			name:         "happy_path",
			args:         []string{"Cal", "Com", "in", "neyjadizwc"},
			wantName:     "Cal Com",
			wantCustomer: "neyjadizwc",
		},
		{
			name:    "missing_in",
			args:    []string{"Cal", "Com"},
			wantErr: true,
		},
		{
			name:    "missing_customer",
			args:    []string{"Cal", "in"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, customer, err := parseCreateArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tt.wantName {
				t.Fatalf("project name mismatch: %q != %q", name, tt.wantName)
			}
			if customer != tt.wantCustomer {
				t.Fatalf("customer mismatch: %q != %q", customer, tt.wantCustomer)
			}
		})
	}
}

func TestCreateCommandRunHappyPath(t *testing.T) {
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	projectIDN := ""
	slug := ""
	verbose := false

	fakeService := &stubCreateService{
		result: deploy.DeployResult{
			ProjectID:   "project-uuid",
			ProjectSlug: "cal-com",
		},
	}

	var added struct {
		path        string
		customerIDN string
		projectIDN  string
		projectID   string
		called      bool
	}

	cmd := &CreateCommand{
		stdout:     io.Discard,
		stderr:     io.Discard,
		console:    console.New(io.Discard, io.Discard, console.WithColors(false)),
		projectIDN: &projectIDN,
		slug:       &slug,
		verbose:    &verbose,
		loadEnv: func() (config.Env, error) {
			return config.Env{OutputRoot: "integrations", SlugPrefix: ""}, nil
		},
		loadCustomers: func(config.Env) (customer.Configuration, error) {
			return customer.Configuration{
				Entries: []customer.Entry{
					{APIKey: "key", HintIDN: "neyjadizwc", Type: "integration"},
				},
			}, nil
		},
		loadRegistry: func() (*state.APIKeyRegistry, error) {
			return state.NewAPIKeyRegistry(), nil
		},
		newSession: func(context.Context, config.Env, customer.Entry, *state.APIKeyRegistry) (*session.Session, error) {
			return &session.Session{
				IDN:          "neyjadizwc",
				CustomerType: "integration",
				Client:       (*platform.Client)(nil),
			}, nil
		},
		newService: func(deploy.DeployClient) createRunner {
			return fakeService
		},
		addProject: func(path, customerIDN, projectIDN, projectID string) error {
			added = struct {
				path        string
				customerIDN string
				projectIDN  string
				projectID   string
				called      bool
			}{
				path:        path,
				customerIDN: customerIDN,
				projectIDN:  projectIDN,
				projectID:   projectID,
				called:      true,
			}
			return nil
		},
	}

	if err := os.MkdirAll(filepath.Join(".newo", "neyjadizwc"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := cmd.Run(context.Background(), []string{"Cal", "Com", "in", "neyjadizwc"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if fakeService.lastReq.ProjectName != "Cal Com" {
		t.Fatalf("project name not forwarded: %q", fakeService.lastReq.ProjectName)
	}
	if fakeService.lastReq.AgentIDN != "CalComAgent" {
		t.Fatalf("agent idn mismatch: %s", fakeService.lastReq.AgentIDN)
	}
	if len(fakeService.lastReq.FlowIDNs) != len(defaultCreateFlows) {
		t.Fatalf("unexpected flow count %d", len(fakeService.lastReq.FlowIDNs))
	}
	if !added.called || added.projectID != "project-uuid" {
		t.Fatalf("addProject not invoked correctly: %+v", added)
	}
}
