package security

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

var key16 = []byte("0123456789abcdef") // AES-128

// TestStateCookie_RoundTrip: every AES key size decodes what it encoded.
func TestStateCookie_RoundTrip(t *testing.T) {
	for _, key := range [][]byte{key16, []byte("0123456789abcdef01234567"), []byte("0123456789abcdef0123456789abcdef")} {
		encoded, err := EncodeStateCookie(key, "random-state", "random-nonce", "random-verifier")
		if err != nil {
			t.Fatalf("%d-byte key: EncodeStateCookie: %v", len(key), err)
		}
		state, nonce, verifier, err := DecodeStateCookie(key, encoded)
		if err != nil || state != "random-state" || nonce != "random-nonce" || verifier != "random-verifier" {
			t.Errorf("%d-byte key: decoded (%q, %q, %q, %v)", len(key), state, nonce, verifier, err)
		}
	}
}

// TestStateCookie_Rejects: expired, tampered, foreign or malformed cookies
// never decode, and keys that are not 16, 24 or 32 bytes are refused both ways.
func TestStateCookie_Rejects(t *testing.T) {
	valid, err := EncodeStateCookie(key16, "state", "nonce", "verifier")
	if err != nil {
		t.Fatalf("EncodeStateCookie: %v", err)
	}
	orig := stateCookieTTL
	stateCookieTTL = -time.Second
	expired, err := EncodeStateCookie(key16, "state", "nonce", "verifier")
	stateCookieTTL = orig
	if err != nil {
		t.Fatalf("EncodeStateCookie: %v", err)
	}
	// Flip a raw byte in the ciphertext (a base64 character flip may hit
	// insignificant bits) so GCM authentication must fail.
	raw, err := base64.RawURLEncoding.DecodeString(valid)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0xff
	tampered := base64.RawURLEncoding.EncodeToString(raw)

	type row struct {
		name   string
		key    []byte
		cookie string
		want   error
	}
	tests := []row{
		{"expired", key16, expired, ErrStateCookieExpired},
		{"tampered ciphertext", key16, tampered, ErrStateCookieTampered},
		{"wrong key", []byte("fedcba9876543210"), valid, ErrStateCookieTampered},
		{"invalid base64", key16, "not-valid-base64!!!", ErrStateCookieTampered},
		{"shorter than the GCM nonce", key16, "AAAAAAAA", ErrStateCookieTampered}, // 6 bytes
	}
	for _, n := range []int{0, 15, 17, 33} {
		tests = append(tests, row{"invalid key length", make([]byte, n), valid, ErrInvalidKeyLength})
		if _, err := EncodeStateCookie(make([]byte, n), "state", "nonce", "verifier"); !errors.Is(err, ErrInvalidKeyLength) {
			t.Errorf("EncodeStateCookie with a %d-byte key: got %v, want ErrInvalidKeyLength", n, err)
		}
	}
	for _, tc := range tests {
		if _, _, _, err := DecodeStateCookie(tc.key, tc.cookie); !errors.Is(err, tc.want) {
			t.Errorf("%s (%d-byte key): got %v, want %v", tc.name, len(tc.key), err, tc.want)
		}
	}
}
