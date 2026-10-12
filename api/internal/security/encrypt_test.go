package security

import (
	"bytes"
	"encoding/hex"
	"errors"
	"slices"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// TestDeriveEncryptionKey pins the derivation (SHA-256 of the secret, 32
// bytes for AES-256): changing it would make every stored ciphertext
// undecryptable.
func TestDeriveEncryptionKey(t *testing.T) {
	t.Parallel()
	for secret, want := range map[string]string{
		"my-secret": "186ef76e9d6a723ecb570d4d9c287487d001e5d35f7ed4a313350a407950318e",
		"secret-b":  "ff492ef788c89b555e6f738b33d2422f57dbb6656af2402155672c5f123a90af",
	} {
		if got := hex.EncodeToString(DeriveEncryptionKey(secret)); got != want {
			t.Errorf("DeriveEncryptionKey(%q) = %s, want %s", secret, got, want)
		}
	}
}

// TestEncryptDecrypt round-trips a value; a random nonce makes two
// encryptions of the same plaintext differ.
func TestEncryptDecrypt(t *testing.T) {
	t.Parallel()
	key := DeriveEncryptionKey("test-secret")
	const plaintext = "hello, world"

	ct1, err1 := Encrypt(plaintext, key)
	ct2, err2 := Encrypt(plaintext, key)
	if err1 != nil || err2 != nil {
		t.Fatalf("Encrypt: %v, %v", err1, err2)
	}
	if ct1 == ct2 {
		t.Error("expected different ciphertexts due to random nonce, got identical values")
	}
	for _, ct := range []string{ct1, ct2} {
		if got, err := Decrypt(ct, key); err != nil || got != plaintext {
			t.Errorf("Decrypt = %q, %v; want %q", got, err, plaintext)
		}
	}
}

func TestDecrypt_Rejects(t *testing.T) {
	t.Parallel()
	key := DeriveEncryptionKey("correct-secret")
	ciphertext, err := Encrypt("sensitive data", key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	for _, tc := range []struct {
		name, input string
		key         []byte
	}{
		{"wrong key", ciphertext, DeriveEncryptionKey("wrong-secret")},
		{"invalid base64", "not-valid-base64!!!", key},
		{"shorter than the GCM nonce", "AQID", key}, // 3 bytes
	} {
		if _, err := Decrypt(tc.input, tc.key); !errors.Is(err, ErrDecryptionFailed) {
			t.Errorf("%s: got %v, want ErrDecryptionFailed", tc.name, err)
		}
	}
}

// TestDecryptWithKeys: a value decrypts under whichever listed key encrypted
// it, however far down the list; a list without that key (or an empty one)
// cannot read it.
func TestDecryptWithKeys(t *testing.T) {
	t.Parallel()
	x, y := DeriveEncryptionKey("key-x"), DeriveEncryptionKey("key-y")
	ciphertext, err := Encrypt("sensitive data", x)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	for _, tc := range []struct {
		name string
		keys [][]byte
		want error
	}{
		{"first key", [][]byte{x, y}, nil},
		{"later key", [][]byte{y, x}, nil},
		{"key absent", [][]byte{y}, ErrDecryptionFailed},
		{"no keys", nil, ErrDecryptionFailed},
	} {
		got, err := DecryptWithKeys(ciphertext, tc.keys)
		if !errors.Is(err, tc.want) || (tc.want == nil && got != "sensitive data") {
			t.Errorf("%s: got (%q, %v), want error %v", tc.name, got, err, tc.want)
		}
	}
}

// TestEncryptionKeys: the first key encrypts new data and is ENCRYPTION_KEY
// when set, else the JWT secret; the JWT secret and the development default
// follow so data written under either still decrypts. Duplicates collapse. An
// emptied JWT secret still counts, since data may have been written under it.
func TestEncryptionKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		cfg  config.Config
		want []string // secrets whose derived keys are expected, in order
	}{
		{"JWT secret only", config.Config{JWTSecret: "jwt"}, []string{"jwt", config.DefaultJWTSecret}},
		{"ENCRYPTION_KEY leads", config.Config{EncryptionKey: "enc", JWTSecret: "jwt"}, []string{"enc", "jwt", config.DefaultJWTSecret}},
		{"default JWT secret", config.Config{JWTSecret: config.DefaultJWTSecret}, []string{config.DefaultJWTSecret}},
		{"ENCRYPTION_KEY equals the JWT secret", config.Config{EncryptionKey: "jwt", JWTSecret: "jwt"}, []string{"jwt", config.DefaultJWTSecret}},
		{"emptied JWT secret", config.Config{}, []string{"", config.DefaultJWTSecret}},
	} {
		var want [][]byte
		for _, secret := range tc.want {
			want = append(want, DeriveEncryptionKey(secret))
		}
		if got := EncryptionKeys(&tc.cfg); !slices.EqualFunc(got, want, bytes.Equal) {
			t.Errorf("%s: got %d keys, want the keys of %q in that order", tc.name, len(got), tc.want)
		}
	}
}
