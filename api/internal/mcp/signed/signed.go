// Package signed authenticates opaque values that leave the MCP server and are
// later handed back by a client.
//
// Two things need this. Attachment download URLs embed an id and an expiry that
// the client could otherwise edit at will. Multi-round-trip write confirmations
// hand the client a RequestState describing a pending database write, which the
// client echoes back on the retry leg — the MCP specification is explicit that
// servers must sign and verify that value. Neither is a secret, so both are
// authenticated rather than encrypted: tampering must be detectable, but the
// contents are already known to the caller.
//
// The server is stateless and horizontally scaled, so a signature is the only
// thing tying a returned value to the replica that issued it.
package signed

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrInvalid reports a value that failed authentication: a bad signature, a
// malformed token, or an expiry that has passed. Callers should treat every
// variant identically and reveal nothing more to the client.
var ErrInvalid = errors.New("invalid signed value")

// Sign returns the hex-encoded HMAC-SHA256 of payload bound to exp.
//
// exp is folded into the MAC rather than appended to it so that an expiry
// cannot be extended without invalidating the signature.
func Sign(key []byte, payload string, exp int64) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	mac.Write([]byte(":exp:"))
	mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks that sig authenticates payload for the given exp and that exp
// has not passed as of now. It returns nil only when both hold.
func Verify(key []byte, payload string, exp int64, sig string, now time.Time) error {
	if exp <= 0 {
		return fmt.Errorf("%w: missing or invalid exp", ErrInvalid)
	}
	if now.Unix() > exp {
		return fmt.Errorf("%w: expired", ErrInvalid)
	}
	// Constant-time compare of the hex strings, to avoid leaking how much of a
	// forged signature was correct.
	if !hmac.Equal([]byte(Sign(key, payload, exp)), []byte(sig)) {
		return fmt.Errorf("%w: signature mismatch", ErrInvalid)
	}
	return nil
}

// Seal JSON-encodes v and returns a self-contained token authenticating it for
// ttl from now. The token is "<base64url(json)>.<exp>.<signature>" — opaque to
// the client, but decodable by any replica holding the same key.
func Seal(key []byte, v any, ttl time.Duration, now time.Time) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("sealing value: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	exp := now.Add(ttl).Unix()
	return encoded + "." + strconv.FormatInt(exp, 10) + "." + Sign(key, encoded, exp), nil
}

// Open authenticates a token produced by Seal and decodes its payload into v,
// which must be a non-nil pointer. It returns an error wrapping ErrInvalid if
// the token is malformed, expired, or not signed by key.
//
// The signature is checked before the body is decoded, so a forged token never
// reaches the JSON decoder.
func Open(key []byte, token string, v any, now time.Time) error {
	encoded, rest, ok := strings.Cut(token, ".")
	if !ok {
		return fmt.Errorf("%w: malformed token", ErrInvalid)
	}
	rawExp, sig, ok := strings.Cut(rest, ".")
	if !ok {
		return fmt.Errorf("%w: malformed token", ErrInvalid)
	}
	exp, err := strconv.ParseInt(rawExp, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: malformed expiry", ErrInvalid)
	}
	if err := Verify(key, encoded, exp, sig, now); err != nil {
		return err
	}
	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("%w: malformed body", ErrInvalid)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: undecodable body", ErrInvalid)
	}
	return nil
}
