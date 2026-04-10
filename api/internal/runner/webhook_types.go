package runner

import "time"

// WebhookPayload is the canonical summary sent on report_completed events.
type WebhookPayload struct {
	Event        string        `json:"event"`
	ProjectID    int64         `json:"project_id"`
	Slug         string        `json:"slug,omitempty"`
	BuildNumber  int           `json:"build_number"`
	DashboardURL string        `json:"dashboard_url,omitempty"`
	Stats        WebhookStats  `json:"stats"`
	Delta        *WebhookDelta `json:"delta,omitempty"`
	CI           *WebhookCI    `json:"ci,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
}

// WebhookStats holds test result statistics for a webhook payload.
type WebhookStats struct {
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	Broken   int     `json:"broken"`
	Skipped  int     `json:"skipped"`
	PassRate float64 `json:"pass_rate"`
}

// WebhookDelta holds the change in metrics compared to the previous build.
type WebhookDelta struct {
	PassRateChange float64 `json:"pass_rate_change"`
	NewFailures    int     `json:"new_failures"`
	FixedTests     int     `json:"fixed_tests"`
}

// WebhookCI holds CI/CD context for a webhook payload.
type WebhookCI struct {
	Provider  string `json:"provider,omitempty"`
	BuildURL  string `json:"build_url,omitempty"`
	Branch    string `json:"branch,omitempty"`
	CommitSHA string `json:"commit_sha,omitempty"`
}
