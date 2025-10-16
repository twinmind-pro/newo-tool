package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/twinmind/newo-tool/internal/llm"
)

// Assert that MockLLMClient implements the llm.LLMClient interface.
var _ llm.LLMClient = (*MockLLMClient)(nil)

// MockLLMClient is a mock implementation of the LLMClient interface for testing.
type MockLLMClient struct {
	GenerateCodeFunc func(prompt string) (string, error)
}

func (m *MockLLMClient) GenerateCode(prompt string) (string, error) {
	if m.GenerateCodeFunc != nil {
		return m.GenerateCodeFunc(prompt)
	}
	return "", errors.New("GenerateCodeFunc not implemented")
}

func TestGenerateCommand_Run(t *testing.T) {
	testCases := []struct {
		name          string
		prompt        string
		llmResponse   string
		llmError      error
		expectedOut   string
		expectedError string
	}{
		{
			name:          "Success",
			prompt:        "set foo to bar",
			llmResponse:   `{% set foo = "bar" %}`,
			expectedOut:   `{% set foo = "bar" %}`,
			expectedError: "",
		},
		{
			name:          "ParserError",
			prompt:        "generate invalid code",
			llmResponse:   `{% set foo = %}`,
			expectedOut:   "",
			expectedError: "failed to parse generated NSL code",
		},
		{
			name:          "NoPrompt",
			prompt:        "",
			expectedOut:   "",
			expectedError: "prompt is required",
		},
		{
			name:          "LLMError",
			prompt:        "any prompt",
			llmError:      errors.New("API limit reached"),
			expectedOut:   "",
			expectedError: "LLM code generation failed: API limit reached",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			cmd := NewGenerateCommand(&stdout, &stderr)
			// Inject the mock client directly into the command.
			cmd.llmClient = &MockLLMClient{
				GenerateCodeFunc: func(prompt string) (string, error) {
					return tc.llmResponse, tc.llmError
				},
			}

			fs := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			cmd.RegisterFlags(fs)

			if err := fs.Parse([]string{"-prompt", tc.prompt}); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			err := cmd.Run(context.Background(), []string{})

			if tc.expectedError != "" {
				if err == nil {
					t.Fatalf("expected an error, but got none")
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Fatalf("expected error to contain %q, but got %q", tc.expectedError, err.Error())
				}
			} else if err != nil {
				t.Fatalf("did not expect an error, but got: %v", err)
			}

			if strings.TrimSpace(stdout.String()) != strings.TrimSpace(tc.expectedOut) {
				t.Fatalf("expected output %q, but got %q", tc.expectedOut, stdout.String())
			}
		})
	}
}
