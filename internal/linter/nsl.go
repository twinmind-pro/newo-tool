package linter

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LintError describes a linting error.
type LintError struct {
	FilePath string
	Line     int
	Message  string
}

func (e LintError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.FilePath, e.Line, e.Message)
}

// LintNSLFiles walks the given root path and lints all .nsl files.
func LintNSLFiles(root string) ([]LintError, error) {
	var errors []LintError

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".nsl") {
			fileErrors, err := lintFile(path)
			if err != nil {
				// Continue linting other files
				errors = append(errors, LintError{FilePath: path, Message: err.Error()})
				return nil
			}
			errors = append(errors, fileErrors...)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return errors, nil
}

func lintFile(filePath string) ([]LintError, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	contentStr := string(content)

	var errors []LintError

	// 1. Check for unbalanced delimiters across the whole file
	delimiters := []struct {
		open  string
		close string
	}{
		{"{{", "}}"},
		{"{%", "%}"},
		{"{#", "#}"},
	}
	for _, d := range delimiters {
		if strings.Count(contentStr, d.open) != strings.Count(contentStr, d.close) {
			errors = append(errors, LintError{
				FilePath: filePath,
				Line:     1,
				Message:  fmt.Sprintf("unbalanced delimiters across file: %s and %s", d.open, d.close),
			})
		}
	}

	// 2. Check for unclosed blocks
	blockErrors := checkBlockTermination(contentStr, filePath)
	errors = append(errors, blockErrors...)

	return errors, nil
}

var blockTagRegex = regexp.MustCompile(`\{%-?\s*(\w+)`)

func checkBlockTermination(content, filePath string) []LintError {
	matches := blockTagRegex.FindAllStringSubmatch(content, -1)
	if matches == nil {
		return nil
	}

	var stack []string
	blockStarters := map[string]bool{"if": true, "for": true, "block": true}
	blockEnders := map[string]string{"endif": "if", "endfor": "for", "endblock": "block"}

	for _, match := range matches {
		tag := match[1]

		if blockStarters[tag] {
			stack = append(stack, tag)
		} else if opener, ok := blockEnders[tag]; ok {
			if len(stack) == 0 {
				return []LintError{{
					FilePath: filePath,
					Line:     1, // Line number is hard to get with regex on whole file, default to 1
					Message:  fmt.Sprintf("unexpected closing tag: %s", tag),
				}}
			}
			if stack[len(stack)-1] != opener {
				return []LintError{{
					FilePath: filePath,
					Line:     1,
					Message:  fmt.Sprintf("mismatched closing tag: expected end for %s, but got %s", stack[len(stack)-1], tag),
				}}
			}
			stack = stack[:len(stack)-1] // Pop
		}
	}

	if len(stack) > 0 {
		return []LintError{{
			FilePath: filePath,
			Line:     1,
			Message:  fmt.Sprintf("unclosed block(s): %s", strings.Join(stack, ", ")),
		}}
	}

	return nil
}
