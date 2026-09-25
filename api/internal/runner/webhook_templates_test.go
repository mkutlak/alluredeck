package runner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestRenderWebhookPayload renders the default Slack/Discord/Teams templates,
// the generic JSON body and a custom template. Every body must be valid JSON
// served as application/json — including optional sections (no delta,
// regressions, digest) and free-form regression messages carrying quotes,
// backslashes and newlines.
func TestRenderWebhookPayload(t *testing.T) {
	custom := `{"project":"{{.Slug}}","build":{{.BuildNumber}}}`
	noDelta := SampleWebhookPayload()
	noDelta.Delta = nil
	regressions := SampleWebhookPayload()
	regressions.Event = "regression_detected"
	regressions.Regressions = []WebhookRegression{
		{FingerprintID: "fp-1", Message: `assertion failed: expected "foo"\nbar`, Category: "product_bug", OccurrenceCount: 3},
		{FingerprintID: "fp-2", Message: "simple message", Category: "test_bug", OccurrenceCount: 1},
	}
	digest := SampleWebhookPayload()
	digest.Event, digest.Delta = "digest", nil
	digest.Digest = &WebhookDigest{
		PeriodStart: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
		RegressionCount: 2,
		Regressions: []WebhookRegression{
			{FingerprintID: "fp-1", Message: `weird "quoted" message`, Category: "infrastructure", OccurrenceCount: 2},
			{FingerprintID: "fp-2", Message: "another one", Category: "to_investigate", OccurrenceCount: 1},
		},
	}

	type row struct {
		name, target string
		tpl          *string
		payload      WebhookPayload
		want, reject []string
	}
	tests := []row{
		{name: "slack", target: "slack", payload: SampleWebhookPayload(), want: []string{"my-project", "Build #42"}},
		// PassRate 95.0 >= 90.0 → green.
		{name: "discord", target: "discord", payload: SampleWebhookPayload(), want: []string{"my-project", "3066993"}},
		{name: "teams", target: "teams", payload: SampleWebhookPayload(), want: []string{"my-project", "AdaptiveCard"}},
		{name: "generic", target: "generic", payload: SampleWebhookPayload()},
		{name: "custom template overrides the default", target: "slack", tpl: &custom, payload: SampleWebhookPayload(),
			want: []string{`"project":"my-project"`, `"build":42`}, reject: []string{"blocks"}},
	}
	for _, target := range []string{"slack", "discord", "teams"} {
		tests = append(tests,
			row{name: target + " without delta", target: target, payload: noDelta},
			row{name: target + " regressions", target: target, payload: regressions, want: []string{"product_bug", "simple message"}},
			row{name: target + " digest", target: target, payload: digest, want: []string{"2026-07-03", "2026-07-04", "infrastructure"}},
		)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, ct, err := RenderWebhookPayload(tc.target, tc.tpl, tc.payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ct != "application/json" || !json.Valid(body) {
				t.Fatalf("content-type %q, want application/json with a valid JSON body: %s", ct, body)
			}
			for _, s := range tc.want {
				if !strings.Contains(string(body), s) {
					t.Errorf("output missing %q: %s", s, body)
				}
			}
			for _, s := range tc.reject {
				if strings.Contains(string(body), s) {
					t.Errorf("output must not contain %q: %s", s, body)
				}
			}
			if tc.target == "generic" {
				var rt WebhookPayload
				if err := json.Unmarshal(body, &rt); err != nil || rt.ProjectID != tc.payload.ProjectID || rt.BuildNumber != tc.payload.BuildNumber {
					t.Errorf("generic body must be the payload JSON, got %s (err %v)", body, err)
				}
			}
		})
	}
}

// TestValidateWebhookTemplate verifies custom templates are checked by
// executing them against SampleWebhookPayload, which must populate every
// nested pointer section so valid templates that reference them are accepted.
func TestValidateWebhookTemplate(t *testing.T) {
	tests := []struct {
		name    string
		tpl     string
		wantErr bool
	}{
		{"valid", `{"project":"{{.ProjectID}}","pass_rate":{{printf "%.1f" .Stats.PassRate}}}`, false},
		{"nested sections", `{{.CI.Provider}} {{.CI.Branch}} {{.CI.CommitSHA}} {{.Delta.NewFailures}} {{.DashboardURL}} {{.Timestamp}}`, false},
		{"unclosed action", `{"x": "{{.ProjectID}`, true},
		{"bad field", `{"x": "{{.NonExistentField}}"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateWebhookTemplate(tt.tpl); (err != nil) != tt.wantErr {
				t.Errorf("ValidateWebhookTemplate(%q) error = %v, want error: %v", tt.tpl, err, tt.wantErr)
			}
		})
	}
}

// TestJSONEscape verifies the jsonescape template func produces content safe
// for embedding inside a literal JSON string.
func TestJSONEscape(t *testing.T) {
	for _, in := range []string{`say "hello"`, `C:\path\to\file`, "line one\nline two", ""} {
		var got string
		if err := json.Unmarshal([]byte(`"`+jsonEscape(in)+`"`), &got); err != nil || got != in {
			t.Errorf("jsonEscape(%q) does not round-trip inside a JSON string: got %q, err %v", in, got, err)
		}
	}
}
