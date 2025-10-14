package main

import "testing"

func TestRunNoArgs(t *testing.T) {
	if err := run(nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}
