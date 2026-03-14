package security

import (
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

func TestExtractClaimsFromMap_Basic(t *testing.T) {
	t.Parallel()

	claims := map[string]any{
		"email":  "alice@example.com",
		"name":   "Alice",
		"groups": []any{"eng", "ops"},
	}

	info := extractClaimsFromMap(claims, "sub-123", zap.NewNop())

	if info.Subject != "sub-123" {
		t.Errorf("Subject = %q, want %q", info.Subject, "sub-123")
	}
	if info.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", info.Email, "alice@example.com")
	}
	if info.Name != "Alice" {
		t.Errorf("Name = %q, want %q", info.Name, "Alice")
	}
	if len(info.Groups) != 2 || info.Groups[0] != "eng" || info.Groups[1] != "ops" {
		t.Errorf("Groups = %v, want [eng ops]", info.Groups)
	}
}

func TestExtractClaimsFromMap_AzureADOverage(t *testing.T) {
	t.Parallel()

	// Azure AD group overage: _claim_names contains "groups" key.
	claims := map[string]any{
		"email": "bob@example.com",
		"name":  "Bob",
		"_claim_names": map[string]any{
			"groups": "_claim_sources",
		},
		"groups": []any{"should-be-ignored"},
	}

	info := extractClaimsFromMap(claims, "sub-azure", zap.NewNop())

	if info.Subject != "sub-azure" {
		t.Errorf("Subject = %q, want %q", info.Subject, "sub-azure")
	}
	if len(info.Groups) != 0 {
		t.Errorf("Groups = %v, want empty (overage detected)", info.Groups)
	}
}

func TestExtractClaimsFromMap_NoGroups(t *testing.T) {
	t.Parallel()

	claims := map[string]any{
		"email": "carol@example.com",
		"name":  "Carol",
	}

	info := extractClaimsFromMap(claims, "sub-carol", zap.NewNop())

	if len(info.Groups) != 0 {
		t.Errorf("Groups = %v, want nil/empty when groups claim absent", info.Groups)
	}
}

func TestExtractClaimsFromMap_NonStringGroups(t *testing.T) {
	t.Parallel()

	// Mixed types: only string entries should be extracted.
	claims := map[string]any{
		"groups": []any{"valid-group", 42, nil, "another-group", true},
	}

	info := extractClaimsFromMap(claims, "sub-mixed", zap.NewNop())

	want := []string{"valid-group", "another-group"}
	if len(info.Groups) != len(want) {
		t.Fatalf("Groups = %v, want %v", info.Groups, want)
	}
	for i, g := range want {
		if info.Groups[i] != g {
			t.Errorf("Groups[%d] = %q, want %q", i, info.Groups[i], g)
		}
	}
}
