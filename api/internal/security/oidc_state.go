package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// statePayload holds the OIDC state, nonce, and PKCE code verifier encrypted in the state cookie.
type statePayload struct {
	State        string `json:"s"`
	Nonce        string `json:"n"`
	CodeVerifier string `json:"v"`
	ExpiresAt    int64  `json:"e"` // unix timestamp
}

// stateCookieTTL is the TTL for OIDC state cookies. It is a variable to allow overriding in tests.
var stateCookieTTL = 5 * time.Minute

// StateCookieTTL returns the current TTL for OIDC state cookies.
func StateCookieTTL() time.Duration { return stateCookieTTL }

// SetStateCookieTTL overrides the state cookie TTL. For testing only.
func SetStateCookieTTL(d time.Duration) { stateCookieTTL = d }

var (
	ErrStateCookieExpired  = errors.New("state cookie has expired")
	ErrStateCookieTampered = errors.New("state cookie is invalid or tampered")
	ErrInvalidKeyLength    = errors.New("AES key must be 16, 24, or 32 bytes")
)

// EncodeStateCookie encrypts state, nonce, and codeVerifier into a base64url-encoded AES-GCM ciphertext.
// The payload expires after stateCookieTTL. The secret must be 16, 24, or 32 bytes for AES-128/192/256.
func EncodeStateCookie(secret []byte, state, nonce, codeVerifier string) (string, error) {
	if err := validateKeyLength(secret); err != nil {
		return "", err
	}

	payload := statePayload{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		ExpiresAt:    time.Now().Add(stateCookieTTL).Unix(),
	}

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal state payload: %w", err)
	}

	block, err := aes.NewCipher(secret)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceBuf := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonceBuf); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonceBuf, nonceBuf, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// DecodeStateCookie decrypts and validates an encoded state cookie.
// Returns the state, nonce, and codeVerifier, or an error if expired or tampered.
func DecodeStateCookie(secret []byte, cookieValue string) (state, nonce, codeVerifier string, err error) {
	if err := validateKeyLength(secret); err != nil {
		return "", "", "", err
	}

	data, err := base64.RawURLEncoding.DecodeString(cookieValue)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", ErrStateCookieTampered, err)
	}

	block, err := aes.NewCipher(secret)
	if err != nil {
		return "", "", "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", "", "", ErrStateCookieTampered
	}

	nonceBuf, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonceBuf, ciphertext, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", ErrStateCookieTampered, err)
	}

	var p statePayload
	if err := json.Unmarshal(plaintext, &p); err != nil {
		return "", "", "", fmt.Errorf("%w: %w", ErrStateCookieTampered, err)
	}

	if time.Now().Unix() > p.ExpiresAt {
		return "", "", "", ErrStateCookieExpired
	}

	return p.State, p.Nonce, p.CodeVerifier, nil
}

func validateKeyLength(key []byte) error {
	n := len(key)
	if n != 16 && n != 24 && n != 32 {
		return fmt.Errorf("%w: got %d bytes", ErrInvalidKeyLength, n)
	}
	return nil
}
