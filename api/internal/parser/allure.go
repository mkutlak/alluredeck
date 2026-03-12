package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseFile parses a single Allure result JSON file and returns a Result.
// Returns an error if the file cannot be read or parsed.
func ParseFile(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read result file %q: %w", path, err)
	}

	var raw allureResultJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse result file %q: %w", path, err)
	}

	// Resolve timing: prefer nested time object (Allure 2), fall back to
	// top-level fields (Allure 3).
	startMs, stopMs, durationMs := raw.Start, raw.Stop, raw.Duration
	if raw.Time != nil {
		startMs = raw.Time.Start
		stopMs = raw.Time.Stop
		durationMs = raw.Time.Duration
	}

	// Resolve status details (nil-safe).
	var statusMsg, statusTrace string
	if raw.StatusDetails != nil {
		statusMsg = raw.StatusDetails.Message
		statusTrace = raw.StatusDetails.Trace
	}

	// Convert labels.
	labels := make([]Label, 0, len(raw.Labels))
	for _, l := range raw.Labels {
		labels = append(labels, Label{Name: l.Name, Value: l.Value})
	}

	// Convert parameters.
	params := make([]Parameter, 0, len(raw.Parameters))
	for _, p := range raw.Parameters {
		params = append(params, Parameter{Name: p.Name, Value: p.Value})
	}

	// Convert attachments.
	attachments := convertAttachments(raw.Attachments)

	// Convert steps recursively.
	steps := convertSteps(raw.Steps, 0)

	return &Result{
		Name:          raw.Name,
		FullName:      raw.FullName,
		HistoryID:     raw.HistoryID,
		Status:        raw.Status,
		StatusMessage: statusMsg,
		StatusTrace:   statusTrace,
		Description:   raw.Description,
		StartMs:       startMs,
		StopMs:        stopMs,
		DurationMs:    durationMs,
		Labels:        labels,
		Parameters:    params,
		Steps:         steps,
		Attachments:   attachments,
	}, nil
}

// ParseDir scans the given directory for files ending in "-result.json"
// and parses each one. Non-matching files are silently skipped.
// Returns all successfully parsed results; parsing errors for individual
// files are returned immediately.
func ParseDir(dir string) ([]*Result, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}

	var results []*Result
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "-result.json") {
			continue
		}
		r, err := ParseFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// convertSteps recursively converts raw step JSON into Step values.
// order is the base index offset for the current level.
func convertSteps(raw []allureStepJSON, order int) []Step {
	steps := make([]Step, 0, len(raw))
	for i, s := range raw {
		var durationMs int64
		if s.Stop > s.Start {
			durationMs = s.Stop - s.Start
		}

		var statusMsg string
		if s.StatusDetails != nil {
			statusMsg = s.StatusDetails.Message
		}

		steps = append(steps, Step{
			Name:          s.Name,
			Status:        s.Status,
			StatusMessage: statusMsg,
			DurationMs:    durationMs,
			Order:         order + i,
			Steps:         convertSteps(s.Steps, 0),
			Attachments:   convertAttachments(s.Attachments),
		})
	}
	return steps
}

// convertAttachments converts raw attachment JSON into Attachment values.
func convertAttachments(raw []allureAttachJSON) []Attachment {
	attachments := make([]Attachment, 0, len(raw))
	for _, a := range raw {
		attachments = append(attachments, Attachment{
			Name:     a.Name,
			Source:   a.Source,
			MimeType: a.Type,
		})
	}
	return attachments
}

// --- internal JSON structs (unexported, used only for decoding) ---

type allureResultJSON struct {
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	HistoryID     string `json:"historyId"`
	Status        string `json:"status"`
	StatusDetails *struct {
		Message string `json:"message"`
		Trace   string `json:"trace"`
	} `json:"statusDetails"`
	Description string `json:"description"`
	// Allure 2: nested time object.
	Time *struct {
		Start    int64 `json:"start"`
		Stop     int64 `json:"stop"`
		Duration int64 `json:"duration"`
	} `json:"time"`
	// Allure 3: top-level time fields.
	Start    int64 `json:"start"`
	Stop     int64 `json:"stop"`
	Duration int64 `json:"duration"`
	Labels   []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"labels"`
	Parameters []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"parameters"`
	Steps       []allureStepJSON   `json:"steps"`
	Attachments []allureAttachJSON `json:"attachments"`
}

type allureStepJSON struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	StatusDetails *struct {
		Message string `json:"message"`
	} `json:"statusDetails"`
	Start       int64              `json:"start"`
	Stop        int64              `json:"stop"`
	Steps       []allureStepJSON   `json:"steps"`
	Attachments []allureAttachJSON `json:"attachments"`
}

type allureAttachJSON struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Type   string `json:"type"`
}
