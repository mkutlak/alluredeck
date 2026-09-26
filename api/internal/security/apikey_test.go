package security

import (
	"strings"
	"testing"
)

// TestGenerateAPIKey: "ald_" + 64 lowercase hex chars, fresh on every call.
func TestGenerateAPIKey(t *testing.T) {
	t.Parallel()
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}
	hexPart, ok := strings.CutPrefix(key, APIKeyPrefix)
	if !ok || len(hexPart) != 64 || strings.Trim(hexPart, "0123456789abcdef") != "" {
		t.Errorf("key %q is not %q + 64 hex chars", key, APIKeyPrefix)
	}
	if key2, err := GenerateAPIKey(); err != nil || key2 == key {
		t.Errorf("expected a distinct second key, got %q (err %v)", key2, err)
	}
}

// TestHashAPIKey pins the stored lookup hash (hex SHA-256 of the full key):
// changing it would orphan every issued key.
func TestHashAPIKey(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{
		"ald_abc123": "3aeefcde042f91e177a03ea76e5574b61cf3122e279b9d2a2add5869a6055728",
		"ald_key2":   "9c50de836b8bcb9cd1cb9b679c05b0125c7baca3929987ccf62a3c30f43ecc19",
	} {
		if got := HashAPIKey(key); got != want {
			t.Errorf("HashAPIKey(%q) = %s, want %s", key, got, want)
		}
	}
}

func TestDisplayPrefix(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{
		"ald_" + strings.Repeat("a", 64): "ald_aaaaaaaa", // "ald_" + 8 hex chars
		"ald_abc":                        "ald_abc",      // shorter keys are shown whole
	} {
		if got := DisplayPrefix(key); got != want {
			t.Errorf("DisplayPrefix(%q) = %q, want %q", key, got, want)
		}
	}
}
