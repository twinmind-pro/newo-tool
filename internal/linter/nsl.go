package linter

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/twinmind/newo-tool/internal/nsl/ast"
	"github.com/twinmind/newo-tool/internal/nsl/lexer"
	"github.com/twinmind/newo-tool/internal/nsl/parser"
)

// ANSI escape codes for colors
const (
	ColorRed   = "\033[31m"
	ColorGreen = "\033[32m"
	ColorReset = "\033[0m"
)

// LintError describes a linting error.
type LintError struct {
	FilePath string
	Line     int
	Message  string
}

func (e LintError) Error() string {
	// Extract the descriptive part of the message (before the actual line content)
	messageParts := strings.SplitN(e.Message, ": ", 2)
	descriptiveMessage := messageParts[0]
	lineContent := ""
	if len(messageParts) > 1 {
		lineContent = messageParts[1]
	}

	// Format: FilePath:Line: DescriptiveMessage
	//         [GREEN]  LineContent[RESET]
	return fmt.Sprintf("%s:%d: %s\n%s  %s%s",
		e.FilePath, e.Line, descriptiveMessage,
		ColorGreen, lineContent, ColorReset)
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
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	var errors []LintError
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	contentBuilder := strings.Builder{}
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		contentBuilder.WriteString(line + "\n")

		// Check for Cyrillic characters
		for _, char := range line {
			if unicode.Is(unicode.Cyrillic, char) {
				errors = append(errors, LintError{
					FilePath: filePath,
					Line:     lineNumber,
					Message:  fmt.Sprintf("Line contains Cyrillic characters: %s", line),
				})
				break // Report only once per line
			}
		}

		// Check for NSL comments
		if strings.Contains(line, "{#") || strings.Contains(line, "#}") {
			errors = append(errors, LintError{
				FilePath: filePath,
				Line:     lineNumber,
				Message:  fmt.Sprintf("Line contains an NSL comment: %s", line),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	contentStr := contentBuilder.String()

	// Existing checks for unbalanced delimiters across the whole file
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

	// Existing checks for unclosed blocks
	blockErrors := checkBlockTermination(contentStr, filePath)
	errors = append(errors, blockErrors...)

	if len(errors) == 0 {
		program, parseErrors := parseNSLProgram(contentStr)
		if len(parseErrors) > 0 {
			return errors, nil
		}

		variableErrors, err := checkUndefinedVariables(filePath, program)
		if err != nil {
			errors = append(errors, LintError{FilePath: filePath, Message: err.Error()})
		} else {
			errors = append(errors, variableErrors...)
		}
	}

	return errors, nil
}

func parseNSLProgram(content string) (*ast.Program, []string) {
	l := lexer.New(content)
	p := parser.New(l)
	program := p.ParseProgram()
	return program, p.Errors()
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
