package runner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRenderWebhookPayload_Slack(t *testing.T) {
	payload := SampleWebhookPayload()
	body, ct, err := RenderWebhookPayload("slack", nil, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("expected content-type application/json, got %q", ct)
	}
	if !json.Valid(body) {
		t.Errorf("output is not valid JSON: %s", body)
	}
	if !strings.Contains(string(body), "my-project") {
		t.Errorf("output missing ProjectID: %s", body)
	}
	if !strings.Contains(string(body), "Build #42") {
		t.Errorf("output missing BuildNumber: %s", body)
	}
}

func TestRenderWebhookPayload_Discord(t *testing.T) {
	payload := SampleWebhookPayload()
	body, ct, err := RenderWebhookPayload("discord", nil, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("expected content-type application/json, got %q", ct)
	}
	if !json.Valid(body) {
		t.Errorf("output is not valid JSON: %s", body)
	}
	// PassRate 95.0 >= 90.0 → green color 3066993
	if !strings.Contains(string(body), "3066993") {
		t.Errorf("expected green color 3066993 for 95%% pass rate: %s", body)
	}
	if !strings.Contains(string(body), "my-project") {
		t.Errorf("output missing ProjectID: %s", body)
	}
}

func TestRenderWebhookPayload_Teams(t *testing.T) {
	payload := SampleWebhookPayload()
	body, ct, err := RenderWebhookPayload("teams", nil, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("expected content-type application/json, got %q", ct)
	}
	if !json.Valid(body) {
		t.Errorf("output is not valid JSON: %s", body)
	}
	if !strings.Contains(string(body), "AdaptiveCard") {
		t.Errorf("output missing AdaptiveCard type: %s", body)
	}
	if !strings.Contains(string(body), "my-project") {
		t.Errorf("output missing ProjectID: %s", body)
	}
}

func TestRenderWebhookPayload_Generic(t *testing.T) {
	payload := SampleWebhookPayload()
	body, ct, err := RenderWebhookPayload("generic", nil, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("expected content-type application/json, got %q", ct)
	}
	if !json.Valid(body) {
		t.Errorf("output is not valid JSON: %s", body)
	}
	// Generic output should be the payload marshalled as JSON.
	var roundtrip WebhookPayload
	if err := json.Unmarshal(body, &roundtrip); err != nil {
		t.Fatalf("failed to unmarshal generic output: %v", err)
	}
	if roundtrip.ProjectID != payload.ProjectID {
		t.Errorf("ProjectID mismatch: got %d, want %d", roundtrip.ProjectID, payload.ProjectID)
	}
	if roundtrip.BuildNumber != payload.BuildNumber {
		t.Errorf("BuildNumber mismatch: got %d, want %d", roundtrip.BuildNumber, payload.BuildNumber)
	}
}

func TestRenderWebhookPayload_CustomTemplate(t *testing.T) {
	payload := SampleWebhookPayload()
	custom := `{"project":"{{.Slug}}","build":{{.BuildNumber}}}`
	body, ct, err := RenderWebhookPayload("slack", &custom, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("expected content-type application/json, got %q", ct)
	}
	if !json.Valid(body) {
		t.Errorf("output is not valid JSON: %s", body)
	}
	if !strings.Contains(string(body), `"project":"my-project"`) {
		t.Errorf("custom template output missing project field: %s", body)
	}
	if !strings.Contains(string(body), `"build":42`) {
		t.Errorf("custom template output missing build field: %s", body)
	}
	// Default Slack structure must NOT appear.
	if strings.Contains(string(body), "blocks") {
		t.Errorf("custom template should override default; found 'blocks' in output: %s", body)
	}
}

func TestRenderWebhookPayload_NoDelta(t *testing.T) {
	payload := SampleWebhookPayload()
	payload.Delta = nil

	for _, tt := range []struct {
		name       string
		targetType string
	}{
		{"slack", "slack"},
		{"discord", "discord"},
		{"teams", "teams"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body, _, err := RenderWebhookPayload(tt.targetType, nil, payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !json.Valid(body) {
				t.Errorf("output is not valid JSON: %s", body)
			}
		})
	}
}

func TestValidateWebhookTemplate_Valid(t *testing.T) {
	tpl := `{"project":"{{.ProjectID}}","pass_rate":{{printf "%.1f" .Stats.PassRate}}}`
	if err := ValidateWebhookTemplate(tpl); err != nil {
		t.Errorf("expected valid template to pass, got error: %v", err)
	}
}

func TestValidateWebhookTemplate_Invalid(t *testing.T) {
	tests := []struct {
		name string
		tpl  string
	}{
		{"unclosed action", `{"x": "{{.ProjectID}`},
		{"bad field", `{"x": "{{.NonExistentField}}"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateWebhookTemplate(tt.tpl); err == nil {
				t.Errorf("expected error for template %q, got nil", tt.tpl)
			}
		})
	}
}

// TestRenderWebhookPayload_Regressions verifies that a non-empty
// payload.Regressions renders a well-formed Regressions section for each
// default target, and that message content containing JSON-hostile
// characters (quotes, backslashes, newlines) does not break the output.
func TestRenderWebhookPayload_Regressions(t *testing.T) {
	payload := SampleWebhookPayload()
	payload.Event = "regression_detected"
	payload.Regressions = []WebhookRegression{
		{
			FingerprintID:   "fp-1",
			Message:         `assertion failed: expected "foo"\nbar`,
			Category:        "product_bug",
			OccurrenceCount: 3,
		},
		{
			FingerprintID:   "fp-2",
			Message:         "simple message",
			Category:        "test_bug",
			OccurrenceCount: 1,
		},
	}

	for _, targetType := range []string{"slack", "discord", "teams"} {
		t.Run(targetType, func(t *testing.T) {
			body, _, err := RenderWebhookPayload(targetType, nil, payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !json.Valid(body) {
				t.Fatalf("output is not valid JSON: %s", body)
			}
			if !strings.Contains(string(body), "product_bug") {
				t.Errorf("output missing regression category: %s", body)
			}
			if !strings.Contains(string(body), "simple message") {
				t.Errorf("output missing second regression message: %s", body)
			}
		})
	}
}

// TestRenderWebhookPayload_Digest verifies that a non-nil payload.Digest
// renders a well-formed digest section for each default target.
func TestRenderWebhookPayload_Digest(t *testing.T) {
	payload := SampleWebhookPayload()
	payload.Event = "digest"
	payload.Delta = nil
	payload.Digest = &WebhookDigest{
		PeriodStart:     time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		PeriodEnd:       time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
		RegressionCount: 2,
		Regressions: []WebhookRegression{
			{FingerprintID: "fp-1", Message: `weird "quoted" message`, Category: "infrastructure", OccurrenceCount: 2},
			{FingerprintID: "fp-2", Message: "another one", Category: "to_investigate", OccurrenceCount: 1},
		},
	}

	for _, targetType := range []string{"slack", "discord", "teams"} {
		t.Run(targetType, func(t *testing.T) {
			body, _, err := RenderWebhookPayload(targetType, nil, payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !json.Valid(body) {
				t.Fatalf("output is not valid JSON: %s", body)
			}
			if !strings.Contains(string(body), "2026-07-03") || !strings.Contains(string(body), "2026-07-04") {
				t.Errorf("output missing digest period: %s", body)
			}
			if !strings.Contains(string(body), "infrastructure") {
				t.Errorf("output missing digest regression category: %s", body)
			}
		})
	}
}

// TestJSONEscape verifies the jsonescape template func produces content
// safe for embedding inside a literal JSON string.
func TestJSONEscape(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"quotes", `say "hello"`},
		{"backslash", `C:\path\to\file`},
		{"newline", "line one\nline two"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := jsonEscape(tt.in)
			// Wrapping in quotes must produce a valid JSON string that
			// round-trips to the original value.
			wrapped := `"` + escaped + `"`
			if !json.Valid([]byte(wrapped)) {
				t.Fatalf("escaped value is not valid inside a JSON string: %s", wrapped)
			}
			var got string
			if err := json.Unmarshal([]byte(wrapped), &got); err != nil {
				t.Fatalf("failed to unmarshal escaped value: %v", err)
			}
			if got != tt.in {
				t.Errorf("round-trip mismatch: got %q, want %q", got, tt.in)
			}
		})
	}
}

func TestSampleWebhookPayload(t *testing.T) {
	p := SampleWebhookPayload()

	if p.ProjectID == 0 {
		t.Error("ProjectID must not be zero")
	}
	if p.BuildNumber == 0 {
		t.Error("BuildNumber must not be zero")
	}
	if p.Event == "" {
		t.Error("Event must not be empty")
	}
	if p.DashboardURL == "" {
		t.Error("DashboardURL must not be empty")
	}
	if p.Stats.Total == 0 {
		t.Error("Stats.Total must not be zero")
	}
	if p.Stats.PassRate == 0 {
		t.Error("Stats.PassRate must not be zero")
	}
	if p.Delta == nil {
		t.Error("Delta must not be nil")
	}
	if p.CI == nil {
		t.Error("CI must not be nil")
	}
	if p.CI != nil {
		if p.CI.Provider == "" {
			t.Error("CI.Provider must not be empty")
		}
		if p.CI.Branch == "" {
			t.Error("CI.Branch must not be empty")
		}
		if p.CI.CommitSHA == "" {
			t.Error("CI.CommitSHA must not be empty")
		}
	}
	if p.Timestamp.IsZero() {
		t.Error("Timestamp must not be zero")
	}
}
