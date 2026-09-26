package security

import (
	"reflect"
	"testing"

	"go.uber.org/zap"
)

// TestPKCEChallenge verifies the S256 computation using the RFC 7636 Appendix B test vector.
// verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
// expected challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
func TestPKCEChallenge(t *testing.T) {
	t.Parallel()

	const (
		verifier      = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		wantChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)

	got := PKCEChallenge(verifier)
	if got != wantChallenge {
		t.Errorf("PKCEChallenge(%q) = %q, want %q", verifier, got, wantChallenge)
	}
}

// TestExtractClaimsFromMap: only string group entries are kept, and an Azure
// AD group overage (_claim_names.groups) drops the groups claim entirely.
func TestExtractClaimsFromMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		claims map[string]any
		want   *OIDCUserInfo
	}{
		{"email, name and groups",
			map[string]any{"email": "alice@example.com", "name": "Alice", "groups": []any{"eng", "ops"}},
			&OIDCUserInfo{Subject: "sub-1", Email: "alice@example.com", Name: "Alice", Groups: []string{"eng", "ops"}}},
		{"Azure AD group overage",
			map[string]any{"email": "bob@example.com", "name": "Bob", "_claim_names": map[string]any{"groups": "_claim_sources"}, "groups": []any{"should-be-ignored"}},
			&OIDCUserInfo{Subject: "sub-1", Email: "bob@example.com", Name: "Bob"}},
		{"non-string groups are skipped",
			map[string]any{"groups": []any{"valid-group", 42, nil, "another-group", true}},
			&OIDCUserInfo{Subject: "sub-1", Groups: []string{"valid-group", "another-group"}}},
	}
	for _, tc := range tests {
		if got := extractClaimsFromMap(tc.claims, "sub-1", zap.NewNop()); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
