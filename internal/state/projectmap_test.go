package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/twinmind/newo-tool/internal/fsutil"
)

func TestProjectMapRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	customer := "TEST"
	if err := os.MkdirAll(filepath.Join(tmp, fsutil.StateDirName), fsutil.DirPerm); err != nil {
		t.Fatalf("mkdir %s: %v", fsutil.StateDirName, err)
	}
	t.Setenv("HOME", tmp)
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	pm := ProjectMap{Projects: map[string]ProjectData{"proj": {ProjectID: "1", Path: "proj"}}}
	if err := SaveProjectMap(customer, pm); err != nil {
		t.Fatalf("SaveProjectMap: %v", err)
	}

	loaded, err := LoadProjectMap(customer)
	if err != nil {
		t.Fatalf("LoadProjectMap: %v", err)
	}
	if len(loaded.Projects) != 1 {
		t.Fatalf("expected project, got %#v", loaded.Projects)
	}
}
