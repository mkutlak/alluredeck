package security

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey_Format(t *testing.T) {
	t.Parallel()
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}
	// "ald_" (4) + 64 hex chars = 68 total
	const wantLen = 68
	if len(key) != wantLen {
		t.Errorf("expected key length %d, got %d: %q", wantLen, len(key), key)
	}
	if !strings.HasPrefix(key, APIKeyPrefix) {
		t.Errorf("expected key to start with %q, got %q", APIKeyPrefix, key)
	}
	// hex portion must be valid hex
	hexPart := key[len(APIKeyPrefix):]
	for _, c := range hexPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("non-hex character %q in key hex portion", c)
		}
	}
}

func TestGenerateAPIKey_Entropy(t *testing.T) {
	t.Parallel()
	key1, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey #1 failed: %v", err)
	}
	key2, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey #2 failed: %v", err)
	}
	if key1 == key2 {
		t.Error("expected two distinct keys, got identical values")
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	t.Parallel()
	key := "ald_abc123"
	hash1 := HashAPIKey(key)
	hash2 := HashAPIKey(key)
	if hash1 != hash2 {
		t.Errorf("HashAPIKey not deterministic: %q vs %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64-char SHA-256 hex digest, got %d chars", len(hash1))
	}
}

func TestHashAPIKey_DifferentKeys(t *testing.T) {
	t.Parallel()
	h1 := HashAPIKey("ald_key1")
	h2 := HashAPIKey("ald_key2")
	if h1 == h2 {
		t.Error("expected distinct hashes for distinct keys")
	}
}

func TestDisplayPrefix_Normal(t *testing.T) {
	t.Parallel()
	key := "ald_" + strings.Repeat("a", 64) // 68 chars total
	prefix := DisplayPrefix(key)
	want := "ald_aaaaaaaa" // "ald_" + 8 hex chars
	if prefix != want {
		t.Errorf("DisplayPrefix = %q, want %q", prefix, want)
	}
}

func TestDisplayPrefix_ShortKey(t *testing.T) {
	t.Parallel()
	short := "ald_abc"
	prefix := DisplayPrefix(short)
	if prefix != short {
		t.Errorf("DisplayPrefix for short key = %q, want original %q", prefix, short)
	}
}

func TestDisplayPrefix_ExactBoundary(t *testing.T) {
	t.Parallel()
	// len(APIKeyPrefix)+8 = 12 exactly
	key := "ald_12345678"
	prefix := DisplayPrefix(key)
	if prefix != key {
		t.Errorf("DisplayPrefix at boundary = %q, want %q", prefix, key)
	}
}
