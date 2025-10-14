package linter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintNSLFiles(t *testing.T) {
	testCases := []struct {
		name        string
		files       map[string]string // filename -> content
		expectError bool
		errorCount  int
		errorMsg    string
	}{
		{
			name: "valid file",
			files: map[string]string{
				"valid.nsl": "{% if true %}\n{{ hello }}\n{% endif %}\n",
			},
			expectError: false,
			errorCount:  0,
		},
		{
			name: "unbalanced delimiters",
			files: map[string]string{
				"unbalanced.nsl": "{{ hello\n",
			},
			expectError: true,
			errorCount:  1,
			errorMsg:    "unbalanced delimiters across file: {{ and }}",
		},
		{
			name: "unclosed block",
			files: map[string]string{
				"unclosed.nsl": "{% if true %}\n",
			},
			expectError: true,
			errorCount:  1,
			errorMsg:    "unclosed block(s): if",
		},
		{
			name: "mismatched block",
			files: map[string]string{
				"mismatched.nsl": "{% if true %}\n{% endfor %}\n",
			},
			expectError: true,
			errorCount:  1,
			errorMsg:    "mismatched closing tag: expected end for if, but got endfor",
		},
		{
			name: "multiple files with errors",
			files: map[string]string{
				"unclosed.nsl":   "{% for item in items %}\n",
				"unbalanced.nsl": "{{ var\n",
			},
			expectError: true,
			errorCount:  2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				filePath := filepath.Join(dir, name)
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write test file: %v", err)
				}
			}

			errors, err := LintNSLFiles(dir)
			if err != nil {
				t.Fatalf("LintNSLFiles failed: %v", err)
			}

			if (len(errors) > 0) != tc.expectError {
				t.Errorf("Expected error to be %v, but got %v errors", tc.expectError, len(errors))
			}

			if len(errors) != tc.errorCount {
				t.Errorf("Expected %d errors, but got %d", tc.errorCount, len(errors))
			}

			if tc.errorMsg != "" && len(errors) > 0 {
				// For simplicity, just check the message of the first error if one is expected
				if errors[0].Message != tc.errorMsg {
					t.Errorf("Expected error message %q, but got %q", tc.errorMsg, errors[0].Message)
				}
			}
		})
	}
}
