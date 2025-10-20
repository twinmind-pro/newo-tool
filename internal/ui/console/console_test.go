package console

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSectionWithoutColour(t *testing.T) {
	var out bytes.Buffer
	w := New(&out, &bytes.Buffer{}, WithColors(false))

	w.Section("Pull")

	got := out.String()
	want := "== Pull ==\n"
	if got != want {
		t.Fatalf("unexpected section output\nwant %q\ngot  %q", want, got)
	}
}

func TestSectionWithColour(t *testing.T) {
	var out bytes.Buffer
	w := New(&out, &bytes.Buffer{}, WithColors(true))

	w.Section("Pull")

	got := out.String()
	if !strings.Contains(got, ansiBlue) || !strings.Contains(got, ansiBold) {
		t.Fatalf("expected coloured section, got %q", got)
	}
	if !strings.HasSuffix(got, ansiReset+"\n") {
		t.Fatalf("expected reset and newline at end, got %q", got)
	}
}

func TestSuccessAndWarnRouting(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	w := New(&out, &errBuf, WithColors(false))

	w.Success("Updated %d project(s)", 2)
	w.Warn("Skipped %s", "foo")

	if !strings.Contains(out.String(), "[+] Updated 2 project(s)") {
		t.Fatalf("success not routed to stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "[!] Skipped foo") {
		t.Fatalf("warn not routed to stderr: %q", errBuf.String())
	}
}

func TestListPrintsItems(t *testing.T) {
	var out bytes.Buffer
	w := New(&out, &bytes.Buffer{}, WithColors(false))

	w.List([]string{"first", "second"})

	got := out.String()
	expectedLines := []string{
		"    - first",
		"    - second",
	}
	for _, line := range expectedLines {
		if !strings.Contains(got, line) {
			t.Fatalf("missing list entry %q in output %q", line, got)
		}
	}
}

func TestNoColorEnvDisablesColours(t *testing.T) {
	old := os.Getenv("NO_COLOR")
	t.Cleanup(func() { _ = os.Setenv("NO_COLOR", old) })
	_ = os.Setenv("NO_COLOR", "1")

	var out bytes.Buffer
	w := New(&out, &bytes.Buffer{}, WithColors(true)) // override should be ignored by NO_COLOR

	w.Success("Done")

	if strings.Contains(out.String(), ansiGreen) {
		t.Fatalf("expected NO_COLOR to disable colour, got %q", out.String())
	}
}

func TestWriteAndPrompt(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	w := New(&out, &errBuf, WithColors(false))

	w.Write("diff\n")
	w.Prompt("Continue? [y/N]: ")
	w.WriteErr("error diff\n")

	got := out.String()
	if !strings.HasPrefix(got, "diff\n") {
		t.Fatalf("expected diff text first, got %q", got)
	}
	if !strings.HasSuffix(got, "Continue? [y/N]: ") {
		t.Fatalf("expected prompt without newline, got %q", got)
	}
	if errBuf.String() != "error diff\n" {
		t.Fatalf("expected stderr write, got %q", errBuf.String())
	}
}
