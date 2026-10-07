package runner

import (
	"regexp"
	"strings"
	"testing"
)

var clusterKeyShape = regexp.MustCompile(`^[0-9a-f]{16}$`)

// TestClusterKey: the key is the failure shape two tests must share to be one
// root cause (normalised message, failure phase, deepest failed step), so the
// REST failures list and diagnose_pipeline group the same tests together.
func TestClusterKey(t *testing.T) {
	const (
		msg   = "connect ECONNREFUSED 10.0.0.1:5432"
		phase = "before_hooks"
		step  = "fixture: db"
	)
	base := ClusterKey(msg, phase, step)

	same := []struct{ name, message, phase, step string }{
		{"IP differs", "connect ECONNREFUSED 192.168.4.77:5432", phase, step},
		{"surrounding whitespace is trimmed", "  \n" + msg + "\t ", phase, step},
	}
	for _, tt := range same {
		if got := ClusterKey(tt.message, tt.phase, tt.step); got != base {
			t.Errorf("%s: key = %q, want %q (same shape)", tt.name, got, base)
		}
	}

	for _, tt := range []struct{ name, a, b string }{
		{"UUID", "order 3f2b8c1e-9a4d-4e6f-8b1a-2c3d4e5f6a7b not found", "order 11111111-2222-3333-4444-555555555555 not found"},
		{"ISO timestamp", "expired at 2026-01-02T03:04:05Z", "expired at 2027-11-12T13:14:15Z"},
		{"5+ digit id", "user 123456 missing", "user 987654321 missing"},
	} {
		if ka, kb := ClusterKey(tt.a, phase, step), ClusterKey(tt.b, phase, step); ka != kb {
			t.Errorf("%s: %q vs %q gave different keys %q, %q", tt.name, tt.a, tt.b, ka, kb)
		}
	}

	differ := []struct{ name, message, phase, step string }{
		{"different phase", msg, "test_body", step},
		{"different last step", msg, phase, "fixture: cache"},
		{"empty last step", msg, phase, ""},
		{"different message", "timeout waiting for locator", phase, step},
	}
	for _, tt := range differ {
		if got := ClusterKey(tt.message, tt.phase, tt.step); got == base {
			t.Errorf("%s: key = %q equals the base key, want a different one", tt.name, got)
		}
	}

	empty := ClusterKey("", phase, step)
	if ws := ClusterKey(" \t\n ", phase, step); ws != empty {
		t.Errorf("whitespace-only key = %q, want the empty-message key %q", ws, empty)
	}
	if empty == base {
		t.Errorf("empty-message key equals a real message's key %q", base)
	}

	for _, k := range []string{base, empty, ClusterKey(msg, "", "")} {
		if !clusterKeyShape.MatchString(k) {
			t.Errorf("key %q is not 16 lowercase hex chars", k)
		}
	}
}

// TestClusterMessage: the deepest failed step's message wins over the test's own
// and falls back to it when the step has none; the result is cut at 500 BYTES
// (not characters, and mid-rune if need be) so that every caller feeding
// ClusterKey from a failing test, diagnose_pipeline and the REST failures list,
// clusters on the same text whatever the script of the message.
func TestClusterMessage(t *testing.T) {
	ascii := strings.Repeat("a", 600)
	cyrillic := strings.Repeat("я", 300) // 2 bytes each: 600 bytes
	midRune := "x" + cyrillic            // the 500th byte falls inside a rune

	for _, tt := range []struct {
		name       string
		step, test string
		want       string
	}{
		{"step message wins", "step", "test", "step"},
		{"falls back to the test message", "", "test", "test"},
		{"both empty", "", "", ""},
		{"short message is unchanged", "boom", "", "boom"},
		{"exactly 500 bytes is unchanged", ascii[:500], "", ascii[:500]},
		{"ASCII step message is cut at 500", ascii, "test", ascii[:500]},
		{"ASCII test message is cut at 500", "", ascii, ascii[:500]},
		{"Cyrillic is cut at 500 bytes, not characters", cyrillic, "", cyrillic[:500]},
		{"the cut may land inside a rune", midRune, "", midRune[:500]},
		{"a long step message beats a long test message", cyrillic, ascii, cyrillic[:500]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := ClusterMessage(tt.step, tt.test)
			if got != tt.want {
				t.Errorf("ClusterMessage = %d bytes (%.24q...), want %d bytes (%.24q...)", len(got), got, len(tt.want), tt.want)
			}
		})
	}
}
