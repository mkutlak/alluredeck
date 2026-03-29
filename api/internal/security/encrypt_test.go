package security

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDeriveEncryptionKey(t *testing.T) {
	t.Parallel()

	t.Run("returns 32 bytes", func(t *testing.T) {
		t.Parallel()
		key := DeriveEncryptionKey("my-secret")
		if len(key) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(key))
		}
	})

	t.Run("same input produces same key", func(t *testing.T) {
		t.Parallel()
		key1 := DeriveEncryptionKey("same-secret")
		key2 := DeriveEncryptionKey("same-secret")
		if !bytes.Equal(key1, key2) {
			t.Error("expected identical keys for identical input")
		}
	})

	t.Run("different input produces different key", func(t *testing.T) {
		t.Parallel()
		key1 := DeriveEncryptionKey("secret-a")
		key2 := DeriveEncryptionKey("secret-b")
		if bytes.Equal(key1, key2) {
			t.Error("expected distinct keys for distinct inputs")
		}
	})
}

func TestEncryptDecrypt(t *testing.T) {
	t.Parallel()

	key := DeriveEncryptionKey("test-secret")
	plaintext := "hello, world"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesDifferentCiphertext(t *testing.T) {
	t.Parallel()

	key := DeriveEncryptionKey("test-secret")
	plaintext := "same plaintext"

	ct1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt #1 failed: %v", err)
	}

	ct2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt #2 failed: %v", err)
	}

	if ct1 == ct2 {
		t.Error("expected different ciphertexts due to random nonce, got identical values")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	t.Parallel()

	key := DeriveEncryptionKey("correct-secret")
	wrongKey := DeriveEncryptionKey("wrong-secret")

	ciphertext, err := Encrypt("sensitive data", key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(ciphertext, wrongKey)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("expected ErrDecryptionFailed, got: %v", err)
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	t.Parallel()

	key := DeriveEncryptionKey("test-secret")
	_, err := Decrypt("not-valid-base64!!!", key)
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("expected ErrDecryptionFailed, got: %v", err)
	}
}

func TestDecryptTooShort(t *testing.T) {
	t.Parallel()

	key := DeriveEncryptionKey("test-secret")
	// Base64-encode a slice that is shorter than the GCM nonce size (12 bytes).
	tooShort := "AQID" // base64 of 3 bytes — well under 12-byte nonce size
	_, err := Decrypt(tooShort, key)
	if err == nil {
		t.Fatal("expected error for too-short ciphertext, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Errorf("expected ErrDecryptionFailed, got: %v", err)
	}
}

func TestEncryptDecryptEmptyString(t *testing.T) {
	t.Parallel()

	key := DeriveEncryptionKey("test-secret")
	plaintext := ""

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Errorf("empty string round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecryptLongText(t *testing.T) {
	t.Parallel()

	key := DeriveEncryptionKey("test-secret")
	plaintext := strings.Repeat("a", 1000) + strings.Repeat("b", 500)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Errorf("long text round-trip mismatch: got length %d, want length %d", len(got), len(plaintext))
	}
}
