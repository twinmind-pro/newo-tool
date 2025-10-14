package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/util"
)

func TestWriteFileWithHashConflictKeepsBaseline(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "conflict.txt")

	if err := os.WriteFile(path, []byte("local-content"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	normalized := filepath.ToSlash(path)
	oldHashes := state.HashStore{
		normalized: util.SHA256Bytes([]byte("previous-remote")),
	}
	newHashes := state.HashStore{}

	cmd := &PullCommand{stderr: &bytes.Buffer{}}

	if err := cmd.writeFileWithHash(oldHashes, newHashes, path, []byte("new-remote"), false); err != nil {
		t.Fatalf("writeFileWithHash conflict: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "local-content" {
		t.Fatalf("file content overwritten, got %q", string(content))
	}

	if got, ok := newHashes[normalized]; !ok {
		t.Fatalf("expected hash entry preserved")
	} else if want := oldHashes[normalized]; got != want {
		t.Fatalf("hash changed in conflict path, want %q got %q", want, got)
	}
}

func TestWriteFileWithHashUpdatesOnSuccess(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	_, _ = w.WriteString("y\n")
	_ = w.Close()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "sync.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	normalized := filepath.ToSlash(path)
	oldHashes := state.HashStore{
		normalized: util.SHA256Bytes([]byte("old")),
	}
	newHashes := state.HashStore{}

	cmd := &PullCommand{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	if err := cmd.writeFileWithHash(oldHashes, newHashes, path, []byte("remote"), false); err != nil {
		t.Fatalf("writeFileWithHash: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "remote" {
		t.Fatalf("expected remote content written, got %q", string(content))
	}

	if got, ok := newHashes[normalized]; !ok {
		t.Fatalf("missing hash entry")
	} else if want := util.SHA256Bytes([]byte("remote")); got != want {
		t.Fatalf("unexpected hash, want %q got %q", want, got)
	}
}
