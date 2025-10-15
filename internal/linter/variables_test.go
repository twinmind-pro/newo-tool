package linter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckUndefinedVariables(t *testing.T) {
	testCases := []struct {
		name        string
		nslContent  string
		metaContent string
		expectError bool
		errorCount  int
		errorMsg    string
	}{
		{
			name:       "valid: variable declared in metadata",
			nslContent: `{{ my_param }}`,
			metaContent: `parameters:
  - name: my_param`,
			expectError: false,
		},
		{
			name:        "invalid: undefined variable",
			nslContent:  `{{ undefined_var }}`,
			metaContent: `parameters: []`,
			expectError: true,
			errorCount:  1,
			errorMsg:    "undefined variable: 'undefined_var' is used but not defined in parameters or in the skill",
		},
		{
			name:       "valid: variable declared in for loop",
			nslContent: `{% for item in items %}{{ item }}{% endfor %}`,
			metaContent: `parameters:
  - name: items`,
			expectError: false,
		},
		{
			name:        "valid: variable declared with set",
			nslContent:  `{% set my_var = 42 %}{{ my_var }}`,
			metaContent: `parameters: []`,
			expectError: false,
		},
		{
			name:       "valid: dot notation on a declared variable",
			nslContent: `{{ user.name }}`,
			metaContent: `parameters:
  - name: user`,
			expectError: false,
		},
		{
			name:        "invalid: dot notation on an undeclared variable",
			nslContent:  `{{ undefined_user.name }}`,
			metaContent: `parameters: []`,
			expectError: true,
			errorCount:  1,
			errorMsg:    "undefined variable: 'undefined_user' is used but not defined in parameters or in the skill",
		},
		{
			name:        "valid: global variable",
			nslContent:  `{% if true %}{{ true }}{% endif %}`,
			metaContent: `parameters: []`,
			expectError: false,
		},
		{
			name:        "invalid: multiple undefined variables",
			nslContent:  `{{ var1 }} and {{ var2 }}`,
			metaContent: `parameters: []`,
			expectError: true,
			errorCount:  2,
		},
		{
			name:        "no metadata file",
			nslContent:  `{{ some_var }}`,
			metaContent: "", // No meta file will be created
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			nslPath := filepath.Join(dir, "test.nsl")
			if err := os.WriteFile(nslPath, []byte(tc.nslContent), 0644); err != nil {
				t.Fatalf("Failed to write nsl file: %v", err)
			}

			if tc.metaContent != "" {
				metaPath := filepath.Join(dir, "test.meta.yaml")
				if err := os.WriteFile(metaPath, []byte(tc.metaContent), 0644); err != nil {
					t.Fatalf("Failed to write meta file: %v", err)
				}
			}

			program, parseErrors := parseNSLProgram(tc.nslContent)
			if len(parseErrors) > 0 {
				t.Fatalf("failed to parse NSL content: %v", parseErrors)
			}

			errors, err := checkUndefinedVariables(nslPath, program)
			if err != nil {
				t.Fatalf("checkUndefinedVariables failed: %v", err)
			}

			if (len(errors) > 0) != tc.expectError {
				t.Errorf("Expected error to be %v, but got %v errors", tc.expectError, len(errors))
			}

			if len(errors) != tc.errorCount {
				t.Errorf("Expected %d errors, but got %d", tc.errorCount, len(errors))
			}

			if tc.errorMsg != "" && len(errors) > 0 {
				// For simplicity, just check the message of the first error if one is expected
				found := false
				for _, e := range errors {
					if e.Message == tc.errorMsg {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error message %q, but got %q", tc.errorMsg, errors[0].Message)
				}
			}
		})
	}
}
