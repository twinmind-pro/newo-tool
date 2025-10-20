package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/state"
)

// MockCommand implements the cli.Command interface for testing purposes.
type MockCommand struct {
	name          string
	summary       string
	registerFlags func(*flag.FlagSet)
	run           func(context.Context, []string) error
}

func (m *MockCommand) Name() string    { return m.name }
func (m *MockCommand) Summary() string { return m.summary }
func (m *MockCommand) RegisterFlags(fs *flag.FlagSet) {
	if m.registerFlags != nil {
		m.registerFlags(fs)
	}
}
func (m *MockCommand) Run(ctx context.Context, args []string) error {
	if m.run != nil {
		return m.run(ctx, args)
	}
	return nil
}

// createTempNewoToml creates a temporary newo.toml file for testing.
func createTempNewoToml(t *testing.T, content string) string {
	tempDir := t.TempDir()
	tomlPath := filepath.Join(tempDir, "newo.toml")
	err := os.WriteFile(tomlPath, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("Failed to create temporary newo.toml: %v", err)
	}
	return tempDir
}

func prepareProjectState(t *testing.T, outputRoot, customerType, customerIDN, projectIDN, slug string) string {
	t.Helper()

	projectDir := fsutil.ExportProjectDir(outputRoot, customerType, customerIDN, slug)
	if err := os.MkdirAll(projectDir, fsutil.DirPerm); err != nil {
		t.Fatalf("create project dir %q: %v", projectDir, err)
	}

	pm := state.ProjectMap{
		Projects: map[string]state.ProjectData{
			projectIDN: {
				ProjectID:  "project-id",
				ProjectIDN: projectIDN,
				Path:       slug,
				Agents:     map[string]state.AgentData{},
			},
		},
	}
	if err := state.SaveProjectMap(customerIDN, pm); err != nil {
		t.Fatalf("save project map for %s: %v", customerIDN, err)
	}
	return projectDir
}

func mustChdir(t *testing.T, dir string) func() {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("switch working directory to %s: %v", dir, err)
	}
	return func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}
}

type tomlCustomer struct {
	idn          string
	apiKey       string
	customerType string
	projects     []string
}

func buildCustomersToml(customers ...tomlCustomer) string {
	var b strings.Builder
	b.WriteString("\n")
	for idx, c := range customers {
		b.WriteString("[[customers]]\n")
		fmt.Fprintf(&b, "  idn = %q\n", c.idn)
		fmt.Fprintf(&b, "  api_key = %q\n", c.apiKey)
		fmt.Fprintf(&b, "  type = %q\n", c.customerType)
		for _, project := range c.projects {
			b.WriteString("  [[customers.projects]]\n")
			fmt.Fprintf(&b, "    idn = %q\n", project)
		}
		if idx < len(customers)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TestMergeCommand_Validation(t *testing.T) {
	tests := []struct {
		name              string
		projectIDN        string
		sourceCustomerIDN string
		targetCustomerIDN string   // Simulates --target-customer flag
		targetWorkspace   []string // Customers that require workspace scaffold
		tomlContent       string
		expectError       bool
		expectedErrorMsg  string
	}{
		{
			name:              "Valid merge: e2e to integration",
			projectIDN:        "test-project",
			sourceCustomerIDN: "e2e-customer",
			targetCustomerIDN: "integration-customer",
			targetWorkspace:   []string{"integration-customer"},
			tomlContent: buildCustomersToml(
				tomlCustomer{
					idn:          "e2e-customer",
					apiKey:       "e2e-key",
					customerType: "e2e",
					projects:     []string{"test-project"},
				},
				tomlCustomer{
					idn:          "integration-customer",
					apiKey:       "integration-key",
					customerType: "integration",
					projects:     []string{"test-project"},
				},
			),
			expectError: false,
		},
		{
			name:              "Auto-detect target: single integration customer",
			projectIDN:        "test-project",
			sourceCustomerIDN: "e2e-customer",
			targetCustomerIDN: "",
			targetWorkspace:   []string{"integration-customer"},
			tomlContent: buildCustomersToml(
				tomlCustomer{
					idn:          "e2e-customer",
					apiKey:       "e2e-key",
					customerType: "e2e",
					projects:     []string{"test-project"},
				},
				tomlCustomer{
					idn:          "integration-customer",
					apiKey:       "integration-key",
					customerType: "integration",
					projects:     []string{"test-project"},
				},
			),
			expectError: false,
		},
		{
			name:              "Invalid source customer type: integration",
			projectIDN:        "test-project",
			sourceCustomerIDN: "integration-customer-source",
			targetCustomerIDN: "integration-customer-target",
			targetWorkspace:   []string{"integration-customer-target"},
			tomlContent: buildCustomersToml(
				tomlCustomer{
					idn:          "integration-customer-source",
					apiKey:       "e2e-key",
					customerType: "integration",
					projects:     []string{"test-project"},
				},
				tomlCustomer{
					idn:          "integration-customer-target",
					apiKey:       "integration-key",
					customerType: "integration",
					projects:     []string{"test-project"},
				},
			),
			expectError:      true,
			expectedErrorMsg: "source customer \"integration-customer-source\" must be of type \"e2e\", but got \"integration\"",
		},
		{
			name:              "Source customer not found",
			projectIDN:        "test-project",
			sourceCustomerIDN: "non-existent-customer",
			targetCustomerIDN: "integration-customer",
			targetWorkspace:   []string{"integration-customer"},
			tomlContent: buildCustomersToml(
				tomlCustomer{
					idn:          "integration-customer",
					apiKey:       "integration-key",
					customerType: "integration",
					projects:     []string{"test-project"},
				},
			),
			expectError:      true,
			expectedErrorMsg: "source customer \"non-existent-customer\" not found in configuration",
		},
		{
			name:              "Target customer not found (explicit)",
			projectIDN:        "test-project",
			sourceCustomerIDN: "e2e-customer",
			targetCustomerIDN: "non-existent-target",
			targetWorkspace:   nil,
			tomlContent: buildCustomersToml(
				tomlCustomer{
					idn:          "e2e-customer",
					apiKey:       "e2e-key",
					customerType: "e2e",
					projects:     []string{"test-project"},
				},
			),
			expectError:      true,
			expectedErrorMsg: "target customer \"non-existent-target\" not found in configuration",
		},
		{
			name:              "Auto-detect target: no integration customer for project",
			projectIDN:        "test-project",
			sourceCustomerIDN: "e2e-customer",
			targetCustomerIDN: "", // Auto-detect
			targetWorkspace:   nil,
			tomlContent: buildCustomersToml(
				tomlCustomer{
					idn:          "e2e-customer",
					apiKey:       "e2e-key",
					customerType: "e2e",
					projects:     []string{"test-project"},
				},
			),
			expectError:      true,
			expectedErrorMsg: "no integration customer found for project \"test-project\"",
		},
		{
			name:              "Auto-detect target: multiple integration customers for project",
			projectIDN:        "test-project",
			sourceCustomerIDN: "e2e-customer",
			targetCustomerIDN: "", // Auto-detect
			targetWorkspace:   nil,
			tomlContent: buildCustomersToml(
				tomlCustomer{
					idn:          "e2e-customer",
					apiKey:       "e2e-key",
					customerType: "e2e",
					projects:     []string{"test-project"},
				},
				tomlCustomer{
					idn:          "integration-customer-1",
					apiKey:       "integration-key-1",
					customerType: "integration",
					projects:     []string{"test-project"},
				},
				tomlCustomer{
					idn:          "integration-customer-2",
					apiKey:       "integration-key-2",
					customerType: "integration",
					projects:     []string{"test-project"},
				},
			),
			expectError:      true,
			expectedErrorMsg: "multiple integration customers found for project \"test-project\": integration-customer-1, integration-customer-2. Please specify with --target-customer flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary newo.toml file for this test
			tempDir := createTempNewoToml(t, tt.tomlContent)
			restore := mustChdir(t, tempDir)
			defer restore()

			if !tt.expectError {
				slug := tt.projectIDN
				outputRoot := fsutil.DefaultCustomersDir
				prepareProjectState(t, outputRoot, "e2e", tt.sourceCustomerIDN, tt.projectIDN, slug)
				for _, customerID := range tt.targetWorkspace {
					prepareProjectState(t, outputRoot, "integration", customerID, tt.projectIDN, slug)
				}
			}

			var stdout, stderr bytes.Buffer
			cmd := NewMergeCommand(&stdout, &stderr)

			// Mock pull and push commands to prevent actual execution during validation tests
			cmd.pullCmdFactory = func(stdout, stderr io.Writer) Command {
				return &MockCommand{
					name: "pull",
					run: func(ctx context.Context, args []string) error {
						return nil // Do nothing
					},
				}
			}
			cmd.pushCmdFactory = func(stdout, stderr io.Writer) Command {
				return &MockCommand{
					name: "push",
					run: func(ctx context.Context, args []string) error {
						return nil // Do nothing
					},
				}
			}

			// Set flags
			fs := flag.NewFlagSet("merge", flag.ContinueOnError)
			cmd.RegisterFlags(fs)
			if tt.targetCustomerIDN != "" {
				_ = fs.Set("target-customer", tt.targetCustomerIDN)
			}
			_ = fs.Set("force", "true")

			// Simulate command line arguments
			args := []string{tt.projectIDN, "from", tt.sourceCustomerIDN}

			err := cmd.Run(context.Background(), args)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error but got none")
				} else if !strings.Contains(err.Error(), tt.expectedErrorMsg) {
					t.Errorf("Expected error message to contain %q, but got %q", tt.expectedErrorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect an error but got: %v", err)
				}
			}
		})
	}
}

func TestMergeCommand_MissingState(t *testing.T) {
	toml := buildCustomersToml(
		tomlCustomer{
			idn:          "e2e-customer",
			apiKey:       "e2e-key",
			customerType: "e2e",
			projects:     []string{"test-project"},
		},
		tomlCustomer{
			idn:          "integration-customer",
			apiKey:       "integration-key",
			customerType: "integration",
			projects:     []string{"test-project"},
		},
	)

	tempDir := createTempNewoToml(t, toml)
	restore := mustChdir(t, tempDir)
	defer restore()

	var stdout, stderr bytes.Buffer
	cmd := NewMergeCommand(&stdout, &stderr)
	cmd.pullCmdFactory = func(stdout, stderr io.Writer) Command {
		return &MockCommand{
			name: "pull",
			run: func(ctx context.Context, args []string) error {
				return nil
			},
		}
	}
	cmd.pushCmdFactory = func(stdout, stderr io.Writer) Command {
		return &MockCommand{
			name: "push",
			run: func(ctx context.Context, args []string) error {
				return nil
			},
		}
	}

	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	cmd.RegisterFlags(fs)
	_ = fs.Set("target-customer", "integration-customer")
	_ = fs.Set("force", "true")

	err := cmd.Run(context.Background(), []string{"test-project", "from", "e2e-customer"})
	if err == nil {
		t.Fatalf("expected error due to missing project state")
	}
	want := "Run 'newo pull --customer e2e-customer --project-idn test-project' first"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to contain %q, got %q", want, err.Error())
	}
}

func TestMergeCommand_CopyProjectFiles(t *testing.T) {
	toml := buildCustomersToml(
		tomlCustomer{
			idn:          "e2e-customer",
			apiKey:       "e2e-key",
			customerType: "e2e",
			projects:     []string{"test-project"},
		},
		tomlCustomer{
			idn:          "integration-customer",
			apiKey:       "integration-key",
			customerType: "integration",
			projects:     []string{"test-project"},
		},
	)

	tempDir := createTempNewoToml(t, toml)
	restore := mustChdir(t, tempDir)
	defer restore()

	outputRoot := fsutil.DefaultCustomersDir
	slug := "test-project"

	sourceDir := prepareProjectState(t, outputRoot, "e2e", "e2e-customer", "test-project", slug)
	targetDir := prepareProjectState(t, outputRoot, "integration", "integration-customer", "test-project", slug)

	sourceFile := filepath.Join(sourceDir, "project.json")
	targetFile := filepath.Join(targetDir, "project.json")

	if err := os.WriteFile(sourceFile, []byte(`{"from":"source"}`), fsutil.FilePerm); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetFile), fsutil.DirPerm); err != nil {
		t.Fatalf("ensure target file dir: %v", err)
	}
	if err := os.WriteFile(targetFile, []byte(`{"from":"target"}`), fsutil.FilePerm); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := NewMergeCommand(&stdout, &stderr)

	cmd.pullCmdFactory = func(stdout, stderr io.Writer) Command {
		return &MockCommand{
			name: "pull",
			run: func(ctx context.Context, args []string) error {
				t.Fatalf("pull should not be called when --no-pull is set")
				return nil
			},
		}
	}
	cmd.pushCmdFactory = func(stdout, stderr io.Writer) Command {
		return &MockCommand{
			name: "push",
			run: func(ctx context.Context, args []string) error {
				t.Fatalf("push should not be called when --no-push is set")
				return nil
			},
		}
	}

	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	cmd.RegisterFlags(fs)
	_ = fs.Set("target-customer", "integration-customer")
	_ = fs.Set("force", "true")
	_ = fs.Set("no-pull", "true")
	_ = fs.Set("no-push", "true")

	if err := cmd.Run(context.Background(), []string{"test-project", "from", "e2e-customer"}); err != nil {
		t.Fatalf("merge command failed: %v", err)
	}

	got, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if string(got) != `{"from":"source"}` {
		t.Fatalf("expected target file to be overwritten, got %s", string(got))
	}
}
