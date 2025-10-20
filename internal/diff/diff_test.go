package diff

import (
	"regexp"
	"strings"
	"testing"
)

func TestFormatProducesAsciiTable(t *testing.T) {
	lines := []Line{
		{Kind: "context", Text: "line 1", LocalLine: 9, RemoteLine: 9},
		{Kind: "del", Text: "old", LocalLine: 10},
		{Kind: "add", Text: "new", RemoteLine: 10},
		{Kind: "context", Text: "line 2", LocalLine: 11, RemoteLine: 11},
	}

	got := Format("file.txt", lines)
	plain := stripANSI(got)
	wantLines := []string{
		"  +-----+-----------------------+",
		"  | diff file.txt (@@ -9 +9 @@) |",
		"  +-----+-----------------------+",
		"  |   9 | line 1                |",
		"  | -10 | old                   |",
		"  | +10 | new                   |",
		"  |  11 | line 2                |",
		"  +-----+-----------------------+",
	}
	want := strings.Join(wantLines, "\n") + "\n"

	if plain != want {
		t.Fatalf("unexpected formatted diff.\nwant:\n%s\ngot:\n%s", want, plain)
	}

	if !strings.Contains(got, redColor) {
		t.Fatalf("expected deletion lines to include red colour code")
	}
	if !strings.Contains(got, greenColor) {
		t.Fatalf("expected addition lines to include green colour code")
	}
}

func TestFormatEmptyLines(t *testing.T) {
	if got := Format("any", nil); got != "" {
		t.Fatalf("expected empty format for nil lines, got %q", got)
	}
	if got := Format("any", []Line{}); got != "" {
		t.Fatalf("expected empty format for empty slice, got %q", got)
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
