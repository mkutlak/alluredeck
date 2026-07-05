package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
	"time"
)

const slackDefaultTemplate = `{
  "blocks": [
    {
      "type": "header",
      "text": {"type": "plain_text", "text": "{{jsonescape .Slug}} — Build #{{.BuildNumber}}"}
    },
    {
      "type": "section",
      "fields": [
        {"type": "mrkdwn", "text": "*Pass Rate:* {{printf "%.1f" .Stats.PassRate}}%"},
        {"type": "mrkdwn", "text": "*Total:* {{.Stats.Total}}"},
        {"type": "mrkdwn", "text": "*Passed:* {{.Stats.Passed}}"},
        {"type": "mrkdwn", "text": "*Failed:* {{.Stats.Failed}}"},
        {"type": "mrkdwn", "text": "*Broken:* {{.Stats.Broken}}"},
        {"type": "mrkdwn", "text": "*Skipped:* {{.Stats.Skipped}}"}
      ]
    }{{if .Delta}},
    {
      "type": "section",
      "fields": [
        {"type": "mrkdwn", "text": "*Pass Rate Δ:* {{printf "%+.1f" .Delta.PassRateChange}}%"},
        {"type": "mrkdwn", "text": "*New Failures:* {{.Delta.NewFailures}}"},
        {"type": "mrkdwn", "text": "*Fixed:* {{.Delta.FixedTests}}"}
      ]
    }{{end}}{{if .Regressions}},
    {
      "type": "section",
      "text": {"type": "mrkdwn", "text": "*Regressions:*\n{{range .Regressions}}• {{jsonescape .Message}} _({{jsonescape .Category}})_\n{{end}}"}
    }{{end}}{{if .Digest}},
    {
      "type": "section",
      "text": {"type": "mrkdwn", "text": "*Digest — {{.Digest.PeriodStart.Format "2006-01-02"}} to {{.Digest.PeriodEnd.Format "2006-01-02"}}:* {{.Digest.RegressionCount}} regression(s)\n{{range .Digest.Regressions}}• {{jsonescape .Message}} _({{jsonescape .Category}})_\n{{end}}"}
    }{{end}}{{if .DashboardURL}},
    {
      "type": "actions",
      "elements": [
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "View Dashboard"},
          "url": "{{.DashboardURL}}"
        }
      ]
    }{{end}}
  ]
}`

const discordDefaultTemplate = `{
  "embeds": [
    {
      "title": "{{jsonescape .Slug}} — Build #{{.BuildNumber}}",
      "color": {{if ge .Stats.PassRate 90.0}}3066993{{else if ge .Stats.PassRate 70.0}}16776960{{else}}15158332{{end}},
      "fields": [
        {"name": "Pass Rate", "value": "{{printf "%.1f" .Stats.PassRate}}%", "inline": true},
        {"name": "Total", "value": "{{.Stats.Total}}", "inline": true},
        {"name": "Failed", "value": "{{.Stats.Failed}}", "inline": true},
        {"name": "Broken", "value": "{{.Stats.Broken}}", "inline": true}{{if .Regressions}},
        {"name": "Regressions", "value": "{{range .Regressions}}{{jsonescape .Message}} ({{jsonescape .Category}})\n{{end}}", "inline": false}{{end}}{{if .Digest}},
        {"name": "Digest Summary", "value": "{{.Digest.PeriodStart.Format "2006-01-02"}} to {{.Digest.PeriodEnd.Format "2006-01-02"}}: {{.Digest.RegressionCount}} regression(s)\n{{range .Digest.Regressions}}{{jsonescape .Message}} ({{jsonescape .Category}})\n{{end}}", "inline": false}{{end}}
      ]{{if .DashboardURL}},
      "url": "{{.DashboardURL}}"{{end}}
    }
  ]
}`

const teamsDefaultTemplate = `{
  "type": "message",
  "attachments": [
    {
      "contentType": "application/vnd.microsoft.card.adaptive",
      "content": {
        "type": "AdaptiveCard",
        "$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
        "version": "1.4",
        "body": [
          {
            "type": "TextBlock",
            "text": "{{jsonescape .Slug}} — Build #{{.BuildNumber}}",
            "weight": "Bolder",
            "size": "Medium"
          },
          {
            "type": "FactSet",
            "facts": [
              {"title": "Pass Rate", "value": "{{printf "%.1f" .Stats.PassRate}}%"},
              {"title": "Total", "value": "{{.Stats.Total}}"},
              {"title": "Failed", "value": "{{.Stats.Failed}}"},
              {"title": "Broken", "value": "{{.Stats.Broken}}"}
            ]
          }{{if .Regressions}},
          {
            "type": "TextBlock",
            "text": "**Regressions:**\n{{range .Regressions}}- {{jsonescape .Message}} ({{jsonescape .Category}})\n{{end}}",
            "wrap": true
          }{{end}}{{if .Digest}},
          {
            "type": "TextBlock",
            "text": "**Digest ({{.Digest.PeriodStart.Format "2006-01-02"}} to {{.Digest.PeriodEnd.Format "2006-01-02"}}):** {{.Digest.RegressionCount}} regression(s)\n{{range .Digest.Regressions}}- {{jsonescape .Message}} ({{jsonescape .Category}})\n{{end}}",
            "wrap": true
          }{{end}}
        ]{{if .DashboardURL}},
        "actions": [
          {
            "type": "Action.OpenUrl",
            "title": "View Dashboard",
            "url": "{{.DashboardURL}}"
          }
        ]{{end}}
      }
    }
  ]
}`

var defaultTemplates = map[string]string{
	"slack":   slackDefaultTemplate,
	"discord": discordDefaultTemplate,
	"teams":   teamsDefaultTemplate,
}

// RenderWebhookPayload renders the webhook body for the given target type.
// If customTemplate is non-nil it is parsed and executed against payload.
// For the "generic" target type with no custom template, the payload is
// marshalled directly as indented JSON.
// Returns the rendered body bytes, the content-type string, and any error.
func RenderWebhookPayload(targetType string, customTemplate *string, payload WebhookPayload) ([]byte, string, error) {
	const contentType = "application/json"

	// Custom template overrides everything.
	if customTemplate != nil {
		body, err := renderTemplate("custom", *customTemplate, payload)
		if err != nil {
			return nil, "", fmt.Errorf("render custom template: %w", err)
		}
		return body, contentType, nil
	}

	// Generic with no custom template — plain JSON marshal.
	if targetType == "generic" {
		body, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, "", fmt.Errorf("marshal generic payload: %w", err)
		}
		return body, contentType, nil
	}

	tplStr, ok := defaultTemplates[targetType]
	if !ok {
		return nil, "", fmt.Errorf("unknown webhook target type: %q", targetType)
	}

	body, err := renderTemplate(targetType, tplStr, payload)
	if err != nil {
		return nil, "", fmt.Errorf("render %s template: %w", targetType, err)
	}
	return body, contentType, nil
}

// ValidateWebhookTemplate parses and executes tplStr against a sample payload
// to surface both syntax and runtime errors early.
func ValidateWebhookTemplate(tplStr string) error {
	sample := SampleWebhookPayload()
	if _, err := renderTemplate("validate", tplStr, sample); err != nil {
		return err
	}
	return nil
}

// SampleWebhookPayload returns a realistic payload suitable for template
// validation and test/preview endpoints.
func SampleWebhookPayload() WebhookPayload {
	passRateChange := 2.5
	branch := "main"
	commitSHA := "abc1234def5678"
	buildURL := "https://ci.example.com/builds/42"

	return WebhookPayload{
		Event:        "report_completed",
		ProjectID:    42,
		Slug:         "my-project",
		BuildNumber:  42,
		DashboardURL: "https://alluredeck.example.com/projects/42/reports/42",
		Stats: WebhookStats{
			Total:    100,
			Passed:   95,
			Failed:   3,
			Broken:   1,
			Skipped:  1,
			PassRate: 95.0,
		},
		Delta: &WebhookDelta{
			PassRateChange: passRateChange,
			NewFailures:    1,
			FixedTests:     3,
		},
		CI: &WebhookCI{
			Provider:  "github-actions",
			BuildURL:  buildURL,
			Branch:    branch,
			CommitSHA: commitSHA,
		},
		Timestamp: time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC),
	}
}

// templateFuncs are made available to every rendered webhook template
// (default and custom).
var templateFuncs = template.FuncMap{
	"jsonescape": jsonEscape,
}

// jsonEscape returns s with JSON string-escape sequences applied (quotes,
// backslashes, control characters) but without the surrounding quotes, so it
// can be embedded inside a literal `"..."` in a template. This is needed for
// free-form content — like defect regression messages, which frequently
// contain quotes or newlines from assertion/stack-trace text — that would
// otherwise produce invalid JSON when interpolated raw.
func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil || len(b) < 2 {
		return ""
	}
	return string(b[1 : len(b)-1])
}

// renderTemplate is a shared helper that parses and executes a named template.
func renderTemplate(name, tplStr string, payload WebhookPayload) ([]byte, error) {
	tpl, err := template.New(name).Funcs(templateFuncs).Parse(tplStr)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, payload); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
