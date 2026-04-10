package parser

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// PlaywrightMeta holds report-level metadata extracted from a Playwright HTML report.
type PlaywrightMeta struct {
	Branch    string
	CommitSHA string
	BuildURL  string
	StartTime int64 // epoch ms
	Duration  int64 // ms
	Stats     PlaywrightStats
}

// PlaywrightStats holds aggregated test counts from a Playwright report.
type PlaywrightStats struct {
	Total      int
	Expected   int
	Unexpected int
	Flaky      int
	Skipped    int
}

// --- internal JSON structs (unexported, used only for decoding) ---

type pwReportJSON struct {
	Metadata struct {
		CI *struct {
			CommitHash string `json:"commitHash"`
			BuildHref  string `json:"buildHref"`
			Branch     string `json:"branch"`
		} `json:"ci"`
		GitCommit *struct {
			Hash   string `json:"hash"`
			Branch string `json:"branch"`
		} `json:"gitCommit"`
	} `json:"metadata"`
	StartTime float64      `json:"startTime"`
	Duration  float64      `json:"duration"`
	Files     []pwFileJSON `json:"files"`
	Stats     pwStatsJSON  `json:"stats"`
}

type pwFileJSON struct {
	FileID   string       `json:"fileId"`
	FileName string       `json:"fileName"`
	Tests    []pwTestJSON `json:"tests"`
}

type pwTestJSON struct {
	TestID      string          `json:"testId"`
	Title       string          `json:"title"`
	ProjectName string          `json:"projectName"`
	Location    *pwLocationJSON `json:"location"`
	Duration    float64         `json:"duration"`
	Annotations []any           `json:"annotations"`
	Tags        []string        `json:"tags"`
	Outcome     string          `json:"outcome"` // "expected", "unexpected", "flaky", "skipped"
	Path        []string        `json:"path"`
	OK          bool            `json:"ok"`
	Results     []pwResultJSON  `json:"results"`
}

type pwLocationJSON struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type pwResultJSON struct {
	Duration    float64           `json:"duration"`
	StartTime   string            `json:"startTime"` // ISO 8601 string
	Retry       int               `json:"retry"`
	Steps       []pwStepJSON      `json:"steps"`
	Errors      []json.RawMessage `json:"errors"`
	Status      string            `json:"status"` // "passed", "failed", "timedOut", "interrupted"
	Attachments []pwAttachJSON    `json:"attachments"`
}

type pwStepJSON struct {
	Title       string         `json:"title"`
	StartTime   string         `json:"startTime"`
	Duration    float64        `json:"duration"`
	Steps       []pwStepJSON   `json:"steps"`
	Attachments []pwAttachJSON `json:"attachments"`
	Error       *pwErrorJSON   `json:"error"`
}

type pwErrorJSON struct {
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

type pwAttachJSON struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Path        string `json:"path"` // e.g. "data/abc123.png"
}

type pwStatsJSON struct {
	Total      int  `json:"total"`
	Expected   int  `json:"expected"`
	Unexpected int  `json:"unexpected"`
	Flaky      int  `json:"flaky"`
	Skipped    int  `json:"skipped"`
	OK         bool `json:"ok"`
}

// Markers for locating the embedded base64 ZIP in Playwright HTML reports.
// Older Playwright versions use a JS variable; v1.59+ uses a <template> element.
const (
	pwBase64Marker   = `window.playwrightReportBase64 = "data:application/zip;base64,`
	pwTemplateMarker = `<template id="playwrightReportBase64">data:application/zip;base64,` //nolint:gosec // not a credential, HTML template marker
)

// ExtractPlaywrightData reads a Playwright HTML report from r, locates the
// embedded base64-encoded ZIP, and returns the raw report.json bytes and a
// map of fileId.json → bytes for all other JSON files in the archive.
func ExtractPlaywrightData(r io.Reader) (reportJSON []byte, fileJSONs map[string][]byte, err error) {
	scanner := bufio.NewScanner(r)
	// Playwright embeds a potentially large base64 string; increase the buffer.
	const maxScanBufSize = 64 * 1024 * 1024 // 64 MiB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScanBufSize)

	var encoded string
	for scanner.Scan() {
		line := scanner.Text()

		// Try the old JS variable format: window.playwrightReportBase64 = "data:...";
		if _, after, ok := strings.Cut(line, pwBase64Marker); ok {
			if before, _, ok := strings.Cut(after, `";`); ok {
				encoded = before
			} else {
				encoded = after
			}
			break
		}

		// Try the template element format (Playwright v1.59+):
		// <template id="playwrightReportBase64">data:application/zip;base64,...</template>
		if _, after, ok := strings.Cut(line, pwTemplateMarker); ok {
			if before, _, ok := strings.Cut(after, `</template>`); ok {
				encoded = before
			} else {
				encoded = after
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan playwright html: %w", err)
	}
	if encoded == "" {
		return nil, nil, fmt.Errorf("playwright report marker not found in HTML")
	}

	zipBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("base64 decode playwright zip: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, nil, fmt.Errorf("open playwright zip: %w", err)
	}

	fileJSONs = make(map[string][]byte)
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("read zip entry %q: %w", f.Name, err)
		}
		if f.Name == "report.json" {
			reportJSON = data
		} else {
			fileJSONs[f.Name] = data
		}
	}

	if reportJSON == nil {
		return nil, nil, fmt.Errorf("report.json not found in playwright zip")
	}
	return reportJSON, fileJSONs, nil
}

// ParsePlaywrightReport converts raw Playwright JSON data into []*Result and
// PlaywrightMeta. fileJSONs maps "fileId.json" to the detailed per-file JSON
// bytes; if a detail file is absent the summary from report.json is used.
func ParsePlaywrightReport(reportJSON []byte, fileJSONs map[string][]byte) ([]*Result, *PlaywrightMeta, error) {
	var report pwReportJSON
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		return nil, nil, fmt.Errorf("parse playwright report.json: %w", err)
	}

	// Build a map of detailed file data keyed by fileId.
	detailFiles := make(map[string]pwFileJSON)
	for name, data := range fileJSONs {
		// The detail file is named "{fileId}.json" and its JSON is a pwFileJSON.
		var detail pwFileJSON
		if err := json.Unmarshal(data, &detail); err != nil {
			continue
		}
		detailFiles[name] = detail
	}

	meta := buildPlaywrightMeta(&report)

	var results []*Result
	for _, file := range report.Files {
		detailKey := file.FileID + ".json"
		var tests []pwTestJSON
		if detail, ok := detailFiles[detailKey]; ok {
			tests = detail.Tests
		} else {
			tests = file.Tests
		}

		for i := range tests {
			r := convertPWTest(file.FileName, &tests[i])
			results = append(results, r)
		}
	}

	return results, meta, nil
}

// buildPlaywrightMeta extracts report-level metadata from a parsed report.
func buildPlaywrightMeta(report *pwReportJSON) *PlaywrightMeta {
	meta := &PlaywrightMeta{
		StartTime: int64(report.StartTime),
		Duration:  int64(report.Duration),
		Stats: PlaywrightStats{
			Total:      report.Stats.Total,
			Expected:   report.Stats.Expected,
			Unexpected: report.Stats.Unexpected,
			Flaky:      report.Stats.Flaky,
			Skipped:    report.Stats.Skipped,
		},
	}

	if report.Metadata.GitCommit != nil {
		meta.CommitSHA = report.Metadata.GitCommit.Hash
		meta.Branch = report.Metadata.GitCommit.Branch
	}
	if report.Metadata.CI != nil {
		if meta.CommitSHA == "" {
			meta.CommitSHA = report.Metadata.CI.CommitHash
		}
		if meta.Branch == "" {
			meta.Branch = report.Metadata.CI.Branch
		}
		meta.BuildURL = report.Metadata.CI.BuildHref
	}

	return meta
}

// convertPWTest maps a single Playwright test entry to a parser.Result.
func convertPWTest(fileName string, test *pwTestJSON) *Result {
	// Build name segments without mutating test.Path.
	nameParts := make([]string, 0, len(test.Path)+1)
	nameParts = append(nameParts, test.Path...)
	nameParts = append(nameParts, test.Title)
	name := strings.Join(nameParts, " > ")

	fullNameParts := make([]string, 0, 1+len(nameParts))
	fullNameParts = append(fullNameParts, fileName)
	fullNameParts = append(fullNameParts, nameParts...)
	fullName := strings.Join(fullNameParts, " > ")

	status := mapPWOutcome(test.Outcome)

	var statusMessage, statusTrace string
	var startMs, stopMs, durationMs int64
	var steps []Step
	var attachments []Attachment

	if len(test.Results) > 0 {
		last := test.Results[len(test.Results)-1]

		// Parse start time.
		if t, err := time.Parse(time.RFC3339Nano, last.StartTime); err == nil {
			startMs = t.UnixMilli()
		}
		durationMs = int64(test.Duration)
		stopMs = startMs + durationMs

		// Extract errors.
		errorStrings := make([]string, 0, len(last.Errors))
		for _, raw := range last.Errors {
			if s := extractErrorString(raw); s != "" {
				errorStrings = append(errorStrings, s)
			}
		}
		if len(errorStrings) > 0 {
			statusMessage = errorStrings[0]
			statusTrace = strings.Join(errorStrings, "\n")
		}

		steps = convertPWSteps(last.Steps, 0)
		attachments = convertPWAttachments(last.Attachments)
	}

	// Build labels.
	labels := make([]Label, 0, len(test.Tags)+3)
	for _, tag := range test.Tags {
		value := strings.TrimPrefix(tag, "@")
		labels = append(labels, Label{Name: "tag", Value: value})
	}
	labels = append(labels,
		Label{Name: "suite", Value: fileName},
		Label{Name: "parentSuite", Value: test.ProjectName},
		Label{Name: "framework", Value: "playwright"},
	)

	return &Result{
		Name:          name,
		FullName:      fullName,
		HistoryID:     test.TestID,
		Status:        status,
		StatusMessage: statusMessage,
		StatusTrace:   statusTrace,
		Description:   "",
		StartMs:       startMs,
		StopMs:        stopMs,
		DurationMs:    durationMs,
		Labels:        labels,
		Parameters:    nil,
		Steps:         steps,
		Attachments:   attachments,
	}
}

// mapPWOutcome maps a Playwright outcome string to an Allure-compatible status.
func mapPWOutcome(outcome string) string {
	switch outcome {
	case "expected", "flaky":
		return "passed"
	case "unexpected":
		return "failed"
	case "skipped":
		return "skipped"
	default:
		return outcome
	}
}

// convertPWSteps recursively converts Playwright step JSON into Step values.
func convertPWSteps(raw []pwStepJSON, order int) []Step {
	steps := make([]Step, 0, len(raw))
	for i, s := range raw {
		stepStatus := "passed"
		var statusMsg string
		if s.Error != nil {
			stepStatus = "failed"
			statusMsg = s.Error.Message
		}
		steps = append(steps, Step{
			Name:          s.Title,
			Status:        stepStatus,
			StatusMessage: statusMsg,
			DurationMs:    int64(s.Duration),
			Order:         order + i,
			Steps:         convertPWSteps(s.Steps, 0),
			Attachments:   convertPWAttachments(s.Attachments),
		})
	}
	return steps
}

// convertPWAttachments converts Playwright attachment JSON into Attachment values.
// It strips the "data/" prefix from the Path field to produce the storage source.
func convertPWAttachments(raw []pwAttachJSON) []Attachment {
	attachments := make([]Attachment, 0, len(raw))
	for _, a := range raw {
		source := strings.TrimPrefix(a.Path, "data/")
		attachments = append(attachments, Attachment{
			Name:     a.Name,
			Source:   source,
			MimeType: a.ContentType,
		})
	}
	return attachments
}

// extractErrorString attempts to extract a human-readable string from a raw
// Playwright error value, which may be a plain string or an object with a
// "message" field.
func extractErrorString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Message
	}
	return string(raw)
}
