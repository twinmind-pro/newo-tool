package formatter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatNSLFile(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expected       string
		expectModified bool
	}{
		{
			name:           "no changes",
			input:          "hello\nworld\n",
			expected:       "hello\nworld\n",
			expectModified: false,
		},
		{
			name:           "trailing whitespace",
			input:          "hello   \nworld\t\n",
			expected:       "hello\nworld\n",
			expectModified: true,
		},
		{
			name:           "one blank line is preserved",
			input:          "hello\n\nworld\n",
			expected:       "hello\n\nworld\n",
			expectModified: false,
		},
		{
			name:           "two blank lines become one",
			input:          "hello\n\n\nworld\n",
			expected:       "hello\n\nworld\n",
			expectModified: true,
		},
		{
			name:           "many blank lines become one",
			input:          "hello\n\n\n\n\nworld\n",
			expected:       "hello\n\nworld\n",
			expectModified: true,
		},
		{
			name:           "combined",
			input:          "hello  \n\n\nworld\t\n\n\n\nfinal\n",
			expected:       "hello\n\nworld\n\nfinal\n",
			expectModified: true,
		},
		{
			name:           "no trailing newline",
			input:          "hello",
			expected:       "hello\n",
			expectModified: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			filePath := filepath.Join(dir, "test.nsl")
			if err := os.WriteFile(filePath, []byte(tc.input), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			modified, err := FormatNSLFile(filePath)
			if err != nil {
				t.Fatalf("FormatNSLFile failed: %v", err)
			}

			if modified != tc.expectModified {
				t.Errorf("Expected modified to be %v, but got %v", tc.expectModified, modified)
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read back test file: %v", err)
			}

			if string(content) != tc.expected {
				t.Errorf("Incorrect formatting:\nExpected:\n%q\nGot:\n%q", tc.expected, string(content))
			}
		})
	}
}
