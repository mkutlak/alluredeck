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

// ResolveAttachments reads the Allure-generated test result files to resolve
// hashed attachment filenames and populate sizes. Allure CLI renames attachment
// files with content-based hashes during report generation (e.g. stdout-001.txt
// becomes a007608853ada3187d97d23b85f1d7d4.txt). This function reads the
// generated data/test-results/*.json files to build the originalFileName →
// {hashedName, contentLength} mapping, then updates each Attachment's Source
// and Size. For Allure 2 (which doesn't rename files), it falls back to
// stat-ing the original filename in the attachments directory.
func ResolveAttachments(results []*Result, reportDataDir string) {
	mapping := buildAttachmentMapping(filepath.Join(reportDataDir, "test-results"))
	attachmentsDir := filepath.Join(reportDataDir, "attachments")

	for _, r := range results {
		resolveAttachmentSlice(r.Attachments, mapping, attachmentsDir)
		for i := range r.Steps {
			resolveStepAttachments(&r.Steps[i], mapping, attachmentsDir)
		}
	}
}

// attachmentMapping holds the resolved filename and size for an attachment.
type attachmentMapping struct {
	HashedSource string
	Size         int64
}

// buildAttachmentMapping reads Allure-generated test result JSON files and
// builds a map from originalFileName to the hashed filename and content length.
func buildAttachmentMapping(testResultsDir string) map[string]attachmentMapping {
	mapping := make(map[string]attachmentMapping)

	entries, err := os.ReadDir(testResultsDir)
	if err != nil {
		return mapping
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(testResultsDir, entry.Name()))
		if err != nil {
			continue
		}
		var result generatedResultJSON
		if json.Unmarshal(data, &result) != nil {
			continue
		}
		for _, att := range result.Attachments {
			if att.Link.OriginalFileName != "" && att.Link.ID != "" {
				mapping[att.Link.OriginalFileName] = attachmentMapping{
					HashedSource: att.Link.ID + att.Link.Ext,
					Size:         att.Link.ContentLength,
				}
			}
		}
	}

	return mapping
}

func resolveAttachmentSlice(atts []Attachment, mapping map[string]attachmentMapping, attachmentsDir string) {
	for i := range atts {
		if m, ok := mapping[atts[i].Source]; ok {
			atts[i].Source = m.HashedSource
			atts[i].Size = m.Size
		} else {
			// Fallback: stat original filename (Allure 2 doesn't rename files).
			info, err := os.Stat(filepath.Join(attachmentsDir, atts[i].Source))
			if err == nil {
				atts[i].Size = info.Size()
			}
		}
	}
}

func resolveStepAttachments(step *Step, mapping map[string]attachmentMapping, attachmentsDir string) {
	resolveAttachmentSlice(step.Attachments, mapping, attachmentsDir)
	for i := range step.Steps {
		resolveStepAttachments(&step.Steps[i], mapping, attachmentsDir)
	}
}

// --- internal JSON structs for Allure-generated test results ---

// generatedResultJSON is a minimal struct for parsing Allure-generated test result files.
type generatedResultJSON struct {
	Attachments []generatedAttachmentJSON `json:"attachments"`
}

// generatedAttachmentJSON represents an attachment entry in a generated test result.
type generatedAttachmentJSON struct {
	Link struct {
		ID               string `json:"id"`
		OriginalFileName string `json:"originalFileName"`
		Ext              string `json:"ext"`
		ContentLength    int64  `json:"contentLength"`
	} `json:"link"`
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
