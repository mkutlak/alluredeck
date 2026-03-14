package security

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

var (
	key16 = []byte("0123456789abcdef")                 // AES-128
	key24 = []byte("0123456789abcdef01234567")         // AES-192
	key32 = []byte("0123456789abcdef0123456789abcdef") // AES-256
)

func TestEncodeDecodeStateCookie_RoundTrip(t *testing.T) {
	keys := []struct {
		name string
		key  []byte
	}{
		{"AES-128 (16 bytes)", key16},
		{"AES-192 (24 bytes)", key24},
		{"AES-256 (32 bytes)", key32},
	}

	for _, tc := range keys {
		t.Run(tc.name, func(t *testing.T) {
			state := "random-state-value"
			nonce := "random-nonce-value"
			codeVerifier := "random-code-verifier"

			encoded, err := EncodeStateCookie(tc.key, state, nonce, codeVerifier)
			if err != nil {
				t.Fatalf("EncodeStateCookie() error = %v", err)
			}
			if encoded == "" {
				t.Fatal("EncodeStateCookie() returned empty string")
			}

			gotState, gotNonce, gotCodeVerifier, err := DecodeStateCookie(tc.key, encoded)
			if err != nil {
				t.Fatalf("DecodeStateCookie() error = %v", err)
			}
			if gotState != state {
				t.Errorf("state = %q, want %q", gotState, state)
			}
			if gotNonce != nonce {
				t.Errorf("nonce = %q, want %q", gotNonce, nonce)
			}
			if gotCodeVerifier != codeVerifier {
				t.Errorf("codeVerifier = %q, want %q", gotCodeVerifier, codeVerifier)
			}
		})
	}
}

func TestDecodeStateCookie_Expired(t *testing.T) {
	// Override TTL to force immediate expiry.
	orig := stateCookieTTL
	stateCookieTTL = -1 * time.Second
	defer func() { stateCookieTTL = orig }()

	encoded, err := EncodeStateCookie(key16, "state", "nonce", "verifier")
	if err != nil {
		t.Fatalf("EncodeStateCookie() error = %v", err)
	}

	_, _, _, err = DecodeStateCookie(key16, encoded)
	if !errors.Is(err, ErrStateCookieExpired) {
		t.Errorf("DecodeStateCookie() error = %v, want ErrStateCookieExpired", err)
	}
}

func TestDecodeStateCookie_TamperedCiphertext(t *testing.T) {
	encoded, err := EncodeStateCookie(key16, "state", "nonce", "verifier")
	if err != nil {
		t.Fatalf("EncodeStateCookie() error = %v", err)
	}

	// Decode to raw bytes, flip a byte in the middle of the ciphertext
	// (past the 12-byte GCM nonce, in the auth tag region), then re-encode.
	// Flipping a character in base64url may fall on non-significant bits;
	// manipulating raw bytes guarantees GCM authentication will fail.
	raw, decErr := base64.RawURLEncoding.DecodeString(encoded)
	if decErr != nil {
		t.Fatalf("failed to decode encoded cookie: %v", decErr)
	}
	raw[len(raw)/2] ^= 0xff
	tampered := base64.RawURLEncoding.EncodeToString(raw)

	_, _, _, err = DecodeStateCookie(key16, tampered)
	if !errors.Is(err, ErrStateCookieTampered) {
		t.Errorf("DecodeStateCookie() error = %v, want ErrStateCookieTampered", err)
	}
}

func TestDecodeStateCookie_WrongKey(t *testing.T) {
	encoded, err := EncodeStateCookie(key16, "state", "nonce", "verifier")
	if err != nil {
		t.Fatalf("EncodeStateCookie() error = %v", err)
	}

	wrongKey := []byte("fedcba9876543210") // different 16-byte key
	_, _, _, err = DecodeStateCookie(wrongKey, encoded)
	if !errors.Is(err, ErrStateCookieTampered) {
		t.Errorf("DecodeStateCookie() with wrong key error = %v, want ErrStateCookieTampered", err)
	}
}

func TestDecodeStateCookie_InvalidBase64(t *testing.T) {
	_, _, _, err := DecodeStateCookie(key16, "not-valid-base64!!!")
	if !errors.Is(err, ErrStateCookieTampered) {
		t.Errorf("DecodeStateCookie() error = %v, want ErrStateCookieTampered", err)
	}
}

func TestDecodeStateCookie_TooShortData(t *testing.T) {
	// Encode a valid base64url string that is shorter than the GCM nonce size (12 bytes).
	short := strings.Repeat("A", 8) // 6 bytes decoded — less than 12-byte nonce
	_, _, _, err := DecodeStateCookie(key16, short)
	if !errors.Is(err, ErrStateCookieTampered) {
		t.Errorf("DecodeStateCookie() error = %v, want ErrStateCookieTampered", err)
	}
}

func TestEncodeStateCookie_InvalidKeyLength(t *testing.T) {
	invalidKeys := [][]byte{
		{},
		make([]byte, 15),
		make([]byte, 17),
		make([]byte, 33),
	}

	for _, key := range invalidKeys {
		_, err := EncodeStateCookie(key, "state", "nonce", "verifier")
		if !errors.Is(err, ErrInvalidKeyLength) {
			t.Errorf("EncodeStateCookie() with %d-byte key error = %v, want ErrInvalidKeyLength", len(key), err)
		}
	}
}

func TestDecodeStateCookie_InvalidKeyLength(t *testing.T) {
	// First encode with a valid key so we have a valid cookie.
	encoded, err := EncodeStateCookie(key16, "state", "nonce", "verifier")
	if err != nil {
		t.Fatalf("EncodeStateCookie() error = %v", err)
	}

	invalidKeys := [][]byte{
		{},
		make([]byte, 15),
		make([]byte, 17),
		make([]byte, 33),
	}

	for _, key := range invalidKeys {
		_, _, _, err := DecodeStateCookie(key, encoded)
		if !errors.Is(err, ErrInvalidKeyLength) {
			t.Errorf("DecodeStateCookie() with %d-byte key error = %v, want ErrInvalidKeyLength", len(key), err)
		}
	}
}
