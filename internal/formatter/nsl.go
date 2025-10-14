package formatter

import (
	"os"
	"regexp"
	"strings"
)

var (
	trailingWhitespaceRegex = regexp.MustCompile(`(?m)[\t ]+$`)
	multipleNewlinesRegex   = regexp.MustCompile(`\n{3,}`)
)

// FormatNSLFile reads an .nsl file, applies formatting rules, and writes the content back.
// It returns true if the file was modified.
func FormatNSLFile(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	originalContent := string(content)

	// 1. Trim trailing whitespace from each line
	formattedContent := trailingWhitespaceRegex.ReplaceAllString(originalContent, "")

	// 2. Collapse 3 or more newlines into 2 (which leaves one blank line)
	formattedContent = multipleNewlinesRegex.ReplaceAllString(formattedContent, "\n\n")

	// 3. Ensure single trailing newline at the end of the file
	formattedContent = strings.TrimSpace(formattedContent) + "\n"

	if formattedContent == originalContent {
		return false, nil
	}

	err = os.WriteFile(filePath, []byte(formattedContent), 0644)
	if err != nil {
		return false, err
	}

	return true, nil
}
