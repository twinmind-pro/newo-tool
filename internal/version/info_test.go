package version

import "testing"

func TestInfoDefaults(t *testing.T) {
	if Version == "" && Commit == "" {
		t.Fatalf("version metadata should be set via ldflags; defaults are empty but test ensures symbol presence")
	}
}
