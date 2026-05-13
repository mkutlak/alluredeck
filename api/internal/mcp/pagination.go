package mcp

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

// EncodeCursor encodes an integer offset into an opaque base64url cursor string.
// The cursor is safe to embed in JSON and URL query parameters.
func EncodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// DecodeCursor decodes a cursor produced by EncodeCursor back to an integer offset.
// Returns an error if the cursor is malformed or does not decode to an integer.
func DecodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor: %w", err)
	}
	offset, err := strconv.Atoi(string(b))
	if err != nil {
		return 0, fmt.Errorf("invalid cursor value: %w", err)
	}
	if offset < 0 {
		return 0, fmt.Errorf("cursor offset must be non-negative")
	}
	return offset, nil
}
