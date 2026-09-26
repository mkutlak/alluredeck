package signed_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/mcp/signed"
)

var testKey = []byte("test-signing-key")

func TestVerify(t *testing.T) {
	t.Parallel()
	now := time.Now()
	exp := now.Add(time.Minute).Unix()
	past := now.Add(-time.Second).Unix()
	sig := signed.Sign(testKey, "attachment:7", exp)

	tests := []struct {
		name    string
		payload string
		exp     int64
		sig     string
		key     []byte
		wantErr bool
	}{
		{"fresh signature", "attachment:7", exp, sig, testKey, false},
		{"different payload", "attachment:8", exp, sig, testKey, true},
		{"different exp", "attachment:7", exp + 1, sig, testKey, true},
		{"garbage signature", "attachment:7", exp, "deadbeef", testKey, true},
		{"foreign key", "attachment:7", exp, sig, []byte("other-key"), true},
		{"expired", "attachment:7", past, signed.Sign(testKey, "attachment:7", past), testKey, true},
		{"non-positive exp", "attachment:7", 0, signed.Sign(testKey, "attachment:7", 0), testKey, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := signed.Verify(tc.key, tc.payload, tc.exp, tc.sig, now); (err != nil) != tc.wantErr {
				t.Fatalf("Verify err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

type payload struct {
	Kind string `json:"kind"`
	N    int    `json:"n"`
}

func TestSealOpen(t *testing.T) {
	t.Parallel()
	now := time.Now()
	want := payload{Kind: "flaky", N: 42}
	token, err := signed.Seal(testKey, want, time.Minute, now)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	var got payload
	if err := signed.Open(testKey, token, &got, now); err != nil || got != want {
		t.Fatalf("Open round trip = %+v, %v; want %+v", got, err, want)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	rejected := []struct {
		name  string
		token string
		key   []byte
		at    time.Time
	}{
		{"swapped body", "eyJraW5kIjoiZmxha3kiLCJuIjo5OTl9." + parts[1] + "." + parts[2], testKey, now},
		{"bumped exp", parts[0] + ".99999999999." + parts[2], testKey, now},
		{"garbage signature", parts[0] + "." + parts[1] + ".deadbeef", testKey, now},
		{"dropped section", parts[0] + "." + parts[1], testKey, now},
		{"empty", "", testKey, now},
		{"foreign key", token, []byte("other-key"), now},
		{"expired", token, testKey, now.Add(2 * time.Minute)},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got payload
			if err := signed.Open(tc.key, tc.token, &got, tc.at); err == nil {
				t.Fatal("Open accepted the token, want error")
			}
		})
	}
}
