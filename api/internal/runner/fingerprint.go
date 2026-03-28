package runner

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compiled regexes for NormalizeMessage — order matters, applied top-to-bottom.
var (
	reIP        = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	reUUID      = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	reISO8601   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[^\s]*`)
	reUnixTS    = regexp.MustCompile(`\b1[0-9]{9,12}\b`)
	reHex       = regexp.MustCompile(`\b0x[0-9a-fA-F]{6,}\b`)
	reAbsPath   = regexp.MustCompile(`(?:/[a-zA-Z0-9._-]+){3,}/([a-zA-Z0-9._-]+)`)
	reLongDigit = regexp.MustCompile(`\b\d{5,}\b`)
)

// Compiled regexes for NormalizeTrace.
var (
	reLineCol  = regexp.MustCompile(`:(\d+):\d+\b`)
	reLineNum  = regexp.MustCompile(`:(\d+)\b`)
	reTracePath = regexp.MustCompile(`(?:/[a-zA-Z0-9._-]+){3,}/([a-zA-Z0-9._-]+)`)
)

// Compiled regexes for CategorizeError.
var re5xx = regexp.MustCompile(`status.?code.*5\d\d|status.*5\d\d`)

// NormalizeMessage strips dynamic values from an error message and truncates to 1000 chars.
// Substitutions are applied in order: IP, UUID, ISO timestamp, Unix timestamp, hex address,
// absolute path (basename only), long numeric ID.
func NormalizeMessage(msg string) string {
	if msg == "" {
		return ""
	}

	msg = reIP.ReplaceAllString(msg, "<IP>")
	msg = reUUID.ReplaceAllString(msg, "<UUID>")
	msg = reISO8601.ReplaceAllString(msg, "<TIMESTAMP>")
	msg = reUnixTS.ReplaceAllString(msg, "<TIMESTAMP>")
	msg = reHex.ReplaceAllString(msg, "<HEX>")
	msg = reAbsPath.ReplaceAllStringFunc(msg, func(m string) string {
		// Return the captured basename group (last path component).
		sub := reAbsPath.FindStringSubmatch(m)
		if len(sub) >= 2 {
			return sub[1]
		}
		return m
	})
	msg = reLongDigit.ReplaceAllString(msg, "<ID>")

	if len(msg) > 1000 {
		msg = msg[:1000]
	}

	return msg
}

// NormalizeTrace normalises a stack trace: keeps first 5 lines, replaces line numbers,
// and strips absolute paths to basename.
func NormalizeTrace(trace string) string {
	if trace == "" {
		return ""
	}

	lines := strings.Split(trace, "\n")
	if len(lines) > 5 {
		lines = lines[:5]
	}

	for i, line := range lines {
		// Replace absolute paths with basename first.
		line = reTracePath.ReplaceAllStringFunc(line, func(m string) string {
			sub := reTracePath.FindStringSubmatch(m)
			if len(sub) >= 2 {
				return sub[1]
			}
			return m
		})
		// Replace :col:line patterns before plain :line.
		line = reLineCol.ReplaceAllString(line, ":<LINE>")
		line = reLineNum.ReplaceAllString(line, ":<LINE>")
		lines[i] = line
	}

	return strings.Join(lines, "\n")
}

// ComputeFingerprint returns the SHA-256 hex digest of normalizedMsg + "\n" + normalizedTrace.
func ComputeFingerprint(normalizedMsg, normalizedTrace string) string {
	h := sha256.Sum256([]byte(normalizedMsg + "\n" + normalizedTrace))
	return fmt.Sprintf("%x", h)
}

// infraKeywords are lowercase substrings that indicate an infrastructure failure.
var infraKeywords = []string{
	"connection refused",
	"connection timed out",
	"connection reset",
	"dns resolution failed",
	"no such host",
	"out of memory",
	"oom killed",
	"disk full",
	"no space left on device",
	"permission denied",
	"socket hang up",
	"econnrefused",
	"etimedout",
	"database is locked",
	"too many connections",
	"certificate",
	"ssl",
	"tls handshake",
}

// testBugKeywords are lowercase substrings that indicate a test setup/teardown problem.
var testBugKeywords = []string{
	"setup failed",
	"teardown failed",
	"fixture",
	"before each",
	"after each",
	"beforeall",
	"afterall",
	"@beforeclass",
	"@afterclass",
	"@beforemethod",
	"conftest",
	"setup_method",
	"teardown_method",
	"nosuchelement",
	"stale element reference",
	"staleelementreference",
	"timeout waiting for",
	"wait_for_selector",
	"element not interactable",
}

// productBugKeywords are lowercase substrings that indicate an application assertion failure.
var productBugKeywords = []string{
	"assertionerror",
	"assert",
	"expected",
	"to equal",
	"to be",
	"assertequal",
	"assertthat",
	"expect(",
	"nullpointerexception",
	"typeerror: cannot read",
	"indexoutofboundsexception",
	"keyerror",
	"attributeerror",
}

// CategorizeError returns a defect category for the given message and trace using
// heuristic keyword matching. The first matching category wins; if nothing matches
// the default is store.DefectCategoryToInvestigate.
func CategorizeError(message, trace string) string {
	combined := strings.ToLower(message + " " + trace)

	for _, kw := range infraKeywords {
		if strings.Contains(combined, kw) {
			return store.DefectCategoryInfrastructure
		}
	}

	for _, kw := range testBugKeywords {
		if strings.Contains(combined, kw) {
			return store.DefectCategoryTestBug
		}
	}

	for _, kw := range productBugKeywords {
		if strings.Contains(combined, kw) {
			return store.DefectCategoryProductBug
		}
	}

	if re5xx.MatchString(combined) {
		return store.DefectCategoryProductBug
	}

	return store.DefectCategoryToInvestigate
}

// FingerprintResult holds the computed fingerprint data and the IDs of test results
// that share the same normalised error signature.
type FingerprintResult struct {
	Hash              string
	NormalizedMessage string
	NormalizedTrace   string
	Category          string
	TestResultIDs     []int64
}

// ComputeFingerprintsForResults groups failed test results by their normalised fingerprint hash.
// For each result the effective message is determined as follows:
//   - If StatusMessage is non-empty, use it.
//   - Else if StatusTrace is non-empty, use the first line of the trace.
//   - Otherwise use "<no message>".
func ComputeFingerprintsForResults(results []store.FailedTestResult) map[string]*FingerprintResult {
	out := make(map[string]*FingerprintResult, len(results))

	for _, r := range results {
		msg := r.StatusMessage
		if msg == "" && r.StatusTrace != "" {
			// Use the first line of the trace as the message.
			msg = strings.SplitN(r.StatusTrace, "\n", 2)[0]
		}
		if msg == "" {
			msg = "<no message>"
		}

		normMsg := NormalizeMessage(msg)
		normTrace := NormalizeTrace(r.StatusTrace)
		hash := ComputeFingerprint(normMsg, normTrace)

		fp, exists := out[hash]
		if !exists {
			fp = &FingerprintResult{
				Hash:              hash,
				NormalizedMessage: normMsg,
				NormalizedTrace:   normTrace,
				Category:          CategorizeError(msg, r.StatusTrace),
			}
			out[hash] = fp
		}
		fp.TestResultIDs = append(fp.TestResultIDs, r.ID)
	}

	return out
}
