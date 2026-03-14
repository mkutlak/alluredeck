package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// APIKeyPrefix is the fixed prefix for all generated API keys.
const APIKeyPrefix = "ald_"

// GenerateAPIKey creates a new API key: "ald_" + 64 hex chars (32 random bytes).
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return APIKeyPrefix + hex.EncodeToString(b), nil
}

// HashAPIKey returns the SHA-256 hex digest of the full key.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// DisplayPrefix extracts the display prefix: "ald_" + first 8 hex chars.
func DisplayPrefix(key string) string {
	if len(key) < len(APIKeyPrefix)+8 {
		return key
	}
	return key[:len(APIKeyPrefix)+8]
}
