package util

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256String computes a SHA-256 digest for a string.
func SHA256String(content string) string {
	return SHA256Bytes([]byte(content))
}

// SHA256Bytes computes a SHA-256 digest for byte content.
func SHA256Bytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
