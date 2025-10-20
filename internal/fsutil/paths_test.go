package fsutil

import (
	"os"
	"testing"
)

func TestExportProjectDirDefaults(t *testing.T) {
	dir := ExportProjectDir("", "default", "default_customer", "project")
	if dir != "default_customer/project" { // Updated expected value
		t.Fatalf("unexpected path: %q", dir)
	}
}

func TestEnsureWorkspaceCreatesDirs(t *testing.T) {
	tmp := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	customer := "ACME"
	if err := EnsureWorkspace(customer); err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}

	if _, err := os.Stat(CustomerStateDir(customer)); err != nil {
		t.Fatalf("state dir missing: %v", err)
	}
}
