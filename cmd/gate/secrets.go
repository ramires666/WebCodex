package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// generateSecret generates a cryptographically secure random string with the given prefix.
func generateSecret(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(buf)
	return prefix + value, nil
}

// hashSecret computes SHA-256 hash of the secret string and returns hex string.
func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// secureCompare performs constant-time string comparison to prevent timing attacks.
func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
