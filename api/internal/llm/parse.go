package llm

import (
	"encoding/json"
	"strings"
)

// summaryPayload is the strict-JSON shape the model is asked to return.
type summaryPayload struct {
	Hypothesis string   `json:"hypothesis"`
	Category   string   `json:"category"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

// parseSummary interprets the raw model text. It first strips any ```json
// fences and tries to decode the strict-JSON shape; on failure it falls back to
// treating the whole text as the hypothesis with a heuristic category.
func parseSummary(raw string) Summary {
	cleaned := stripFences(raw)

	var p summaryPayload
	if err := json.Unmarshal([]byte(cleaned), &p); err == nil && strings.TrimSpace(p.Hypothesis) != "" {
		return Summary{
			Hypothesis: strings.TrimSpace(p.Hypothesis),
			Category:   normalizeCategory(p.Category),
			Confidence: normalizeConfidence(p.Confidence),
			Evidence:   p.Evidence,
		}
	}

	// Fallback: the model did not return parseable JSON. Use the whole text as
	// the hypothesis and guess a category from keywords.
	text := strings.TrimSpace(raw)
	return Summary{
		Hypothesis: text,
		Category:   heuristicCategory(text),
	}
}

// stripFences removes a leading/trailing Markdown code fence (```json … ```)
// so a fenced JSON payload can be decoded.
func stripFences(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return t
	}
	// Drop the opening fence line (``` or ```json).
	if nl := strings.IndexByte(t, '\n'); nl >= 0 {
		t = t[nl+1:]
	} else {
		t = strings.TrimPrefix(t, "```")
	}
	// Drop the closing fence.
	if idx := strings.LastIndex(t, "```"); idx >= 0 {
		t = t[:idx]
	}
	return strings.TrimSpace(t)
}

// normalizeCategory lower-cases, trims, and validates the category label. An
// unrecognized value is returned as-is (trimmed/lower-cased) so the raw label
// is still visible to the UI; it is a display-only hint, never an action.
func normalizeCategory(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.Trim(s, ".\"' \t\n")
	switch s {
	case "test_bug", "product_bug", "infrastructure", "flake":
		return s
	}
	return s
}

// normalizeConfidence constrains the confidence to the allowed set; anything
// else becomes "".
func normalizeConfidence(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	default:
		return ""
	}
}

// heuristicCategory guesses a category from keywords in free text, used only on
// the JSON-parse fallback path.
func heuristicCategory(text string) string {
	l := strings.ToLower(text)
	switch {
	case strings.Contains(l, "timeout") || strings.Contains(l, "timed out") ||
		strings.Contains(l, "connection") || strings.Contains(l, "network") ||
		strings.Contains(l, "infrastructure") || strings.Contains(l, "dns"):
		return "infrastructure"
	case strings.Contains(l, "flaky") || strings.Contains(l, "flake") ||
		strings.Contains(l, "intermittent"):
		return "flake"
	default:
		return ""
	}
}
