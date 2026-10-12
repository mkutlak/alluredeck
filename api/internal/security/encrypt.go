package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// ErrDecryptionFailed is returned when ciphertext cannot be decrypted.
var ErrDecryptionFailed = errors.New("decryption failed")

// DeriveEncryptionKey derives a 32-byte AES-256 key from the given secret using SHA-256.
func DeriveEncryptionKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// EncryptionKeys returns the AES keys stored data may be encrypted under, the
// one to encrypt new data with first: ENCRYPTION_KEY when set, else the JWT
// secret. The JWT secret and the development default follow, so data written
// before ENCRYPTION_KEY was set, or while the secret was still the default,
// stays readable. Duplicates are dropped.
func EncryptionKeys(cfg *config.Config) [][]byte {
	var secrets []string
	if cfg.EncryptionKey != "" {
		secrets = append(secrets, cfg.EncryptionKey)
	}
	secrets = append(secrets, cfg.JWTSecret, config.DefaultJWTSecret)

	keys := make([][]byte, 0, len(secrets))
	for i, secret := range secrets {
		if !slices.Contains(secrets[:i], secret) {
			keys = append(keys, DeriveEncryptionKey(secret))
		}
	}
	return keys
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64-encoded ciphertext.
// The nonce is prepended to the ciphertext before encoding.
func Encrypt(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decodes a base64-encoded ciphertext and decrypts it using AES-256-GCM.
func Decrypt(encoded string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: invalid base64", ErrDecryptionFailed)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("%w: ciphertext too short", ErrDecryptionFailed)
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrDecryptionFailed, err)
	}

	return string(plaintext), nil
}

// DecryptWithKeys decrypts encoded with the first key that opens it. GCM
// authentication rejects every other key, so a wrong key never yields garbage.
// It returns ErrDecryptionFailed when no key opens the value.
func DecryptWithKeys(encoded string, keys [][]byte) (string, error) {
	for _, key := range keys {
		plain, err := Decrypt(encoded, key)
		if !errors.Is(err, ErrDecryptionFailed) {
			return plain, err
		}
	}
	return "", ErrDecryptionFailed
}
