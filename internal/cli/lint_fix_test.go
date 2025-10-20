package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twinmind/newo-tool/internal/linter"
	"github.com/twinmind/newo-tool/internal/ui/console"
)

func TestFixNSLComment_RemovesEntireCommentLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.nsl")
	original := "{# comment #}\nkeep\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	issue := linter.LintError{
		FilePath: filepath.ToSlash(path),
		Line:     1,
		Message:  "Line contains an NSL comment",
		Snippet:  "{# comment #}",
		Severity: linter.SeverityWarning,
	}

	changed, err := fixNSLComment(issue)
	if err != nil {
		t.Fatalf("fixNSLComment: %v", err)
	}
	if !changed {
		t.Fatalf("expected change, got none")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	expected := "keep\n"
	if string(data) != expected {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestFixNSLComment_TrimsTrailingComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.nsl")
	original := "value {{ foo }} #}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	issue := linter.LintError{
		FilePath: filepath.ToSlash(path),
		Line:     1,
		Message:  "Line contains an NSL comment",
		Snippet:  "value {{ foo }} #}",
		Severity: linter.SeverityWarning,
	}

	changed, err := fixNSLComment(issue)
	if err != nil {
		t.Fatalf("fixNSLComment: %v", err)
	}
	if !changed {
		t.Fatalf("expected change, got none")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	expected := "value {{ foo }}\n"
	if string(data) != expected {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestApplyFixesInteractive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.nsl")
	original := "{# comment #}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	issue := linter.LintError{
		FilePath: filepath.ToSlash(path),
		Line:     1,
		Message:  "Line contains an NSL comment",
		Snippet:  "{# comment #}",
		Severity: linter.SeverityWarning,
	}

	grouped := map[string][]linter.LintError{
		filepath.ToSlash(path): {issue},
	}

	cmd := &LintCommand{
		stdout:  io.Discard,
		stderr:  io.Discard,
		console: console.New(io.Discard, io.Discard, console.WithColors(false)),
		input:   strings.NewReader("y\n"),
	}

	modified, err := cmd.applyFixes(grouped)
	if err != nil {
		t.Fatalf("applyFixes: %v", err)
	}
	if !modified {
		t.Fatalf("expected modifications")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "" {
		t.Fatalf("expected empty file, got %q", string(data))
	}
}

func TestIsFixableIssue(t *testing.T) {
	issue := linter.LintError{
		Message: "Line contains an NSL comment",
		Snippet: "{# comment #}",
	}
	if !isFixableIssue(issue) {
		t.Fatalf("expected fixable issue")
	}

	issue.Snippet = ""
	if isFixableIssue(issue) {
		t.Fatalf("expected non-fixable issue")
	}

	issue = linter.LintError{
		Message: "Undefined variable",
		Snippet: "foo",
	}
	if isFixableIssue(issue) {
		t.Fatalf("expected non-fixable for different message")
	}
}
