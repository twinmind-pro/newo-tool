package util

import "testing"

func TestSHA256StringStable(t *testing.T) {
	const input = "hello"
	const expected = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got := SHA256String(input); got != expected {
		t.Fatalf("unexpected hash: %q", got)
	}
	bytesHash := SHA256Bytes([]byte(input))
	if bytesHash != expected {
		t.Fatalf("SHA256Bytes mismatch: %q", bytesHash)
	}
}
