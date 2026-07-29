package signed_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/mcp/signed"
)

var testKey = []byte("test-signing-key")

func TestVerifyAcceptsFreshSignature(t *testing.T) {
	t.Parallel()
	now := time.Now()
	exp := now.Add(time.Minute).Unix()
	sig := signed.Sign(testKey, "attachment:7", exp)

	if err := signed.Verify(testKey, "attachment:7", exp, sig, now); err != nil {
		t.Fatalf("Verify on a fresh signature: %v", err)
	}
}

func TestVerifyRejectsTamperedInputs(t *testing.T) {
	t.Parallel()
	now := time.Now()
	exp := now.Add(time.Minute).Unix()
	sig := signed.Sign(testKey, "attachment:7", exp)

	tests := []struct {
		name    string
		payload string
		exp     int64
		sig     string
		key     []byte
	}{
		{"different payload", "attachment:8", exp, sig, testKey},
		{"different exp", "attachment:7", exp + 1, sig, testKey},
		{"garbage signature", "attachment:7", exp, "deadbeef", testKey},
		{"foreign key", "attachment:7", exp, sig, []byte("other-key")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := signed.Verify(tc.key, tc.payload, tc.exp, tc.sig, now); err == nil {
				t.Fatal("Verify accepted a tampered input, want error")
			}
		})
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	t.Parallel()
	now := time.Now()
	exp := now.Add(-time.Second).Unix()
	sig := signed.Sign(testKey, "attachment:7", exp)

	if err := signed.Verify(testKey, "attachment:7", exp, sig, now); err == nil {
		t.Fatal("Verify accepted an expired signature, want error")
	}
}

func TestVerifyRejectsNonPositiveExp(t *testing.T) {
	t.Parallel()
	now := time.Now()
	if err := signed.Verify(testKey, "attachment:7", 0, signed.Sign(testKey, "attachment:7", 0), now); err == nil {
		t.Fatal("Verify accepted exp=0, want error")
	}
}

type payload struct {
	Kind string `json:"kind"`
	N    int    `json:"n"`
}

func TestSealOpenRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now()
	want := payload{Kind: "flaky", N: 42}

	token, err := signed.Seal(testKey, want, time.Minute, now)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	var got payload
	if err := signed.Open(testKey, token, &got, now); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestOpenRejectsTamperedToken(t *testing.T) {
	t.Parallel()
	now := time.Now()
	token, err := signed.Seal(testKey, payload{Kind: "flaky", N: 42}, time.Minute, now)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}

	tests := map[string]string{
		"swapped body":      "eyJraW5kIjoiZmxha3kiLCJuIjo5OTl9." + parts[1] + "." + parts[2],
		"bumped exp":        parts[0] + ".99999999999." + parts[2],
		"garbage signature": parts[0] + "." + parts[1] + ".deadbeef",
		"dropped section":   parts[0] + "." + parts[1],
		"empty":             "",
	}
	for name, tok := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var got payload
			if err := signed.Open(testKey, tok, &got, now); err == nil {
				t.Fatalf("Open accepted a tampered token (%s), want error", name)
			}
		})
	}
}

func TestOpenRejectsForeignKey(t *testing.T) {
	t.Parallel()
	now := time.Now()
	token, err := signed.Seal(testKey, payload{Kind: "flaky", N: 42}, time.Minute, now)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	var got payload
	if err := signed.Open([]byte("other-key"), token, &got, now); err == nil {
		t.Fatal("Open accepted a token signed with a different key, want error")
	}
}

func TestOpenRejectsExpiredToken(t *testing.T) {
	t.Parallel()
	now := time.Now()
	token, err := signed.Seal(testKey, payload{Kind: "flaky", N: 42}, time.Minute, now)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	var got payload
	if err := signed.Open(testKey, token, &got, now.Add(2*time.Minute)); err == nil {
		t.Fatal("Open accepted an expired token, want error")
	}
}
