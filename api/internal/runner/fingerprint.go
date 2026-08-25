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
	reLineCol   = regexp.MustCompile(`:(\d+):\d+\b`)
	reLineNum   = regexp.MustCompile(`:(\d+)\b`)
	reTracePath = regexp.MustCompile(`(?:/[a-zA-Z0-9._-]+){3,}/([a-zA-Z0-9._-]+)`)
)

// re5xx matches a 5xx HTTP code introduced by an HTTP-ish anchor word, so bare
// forms ("HTTP 503", "code: 502", "response 504") are caught alongside the
// explicit "status code: 500". Applied to the lowercased message, so the
// pattern carries no case-insensitivity flag. Four rules keep it honest:
//
//   - Anchors carry a LEADING word boundary, so "decode"/"encoded" can no
//     longer smuggle in the "code" anchor. There is no trailing boundary
//     (except on "code" itself, which needs one for the same reason) because
//     clients print "statusCode: 500" and "status_code=503" glued to the
//     anchor. Bare "failed" is not an anchor at all: the 500 in "test failed
//     after 500 ms" is a duration, not a status.
//   - The gap may not contain digits, so an intervening number breaks the
//     association between an anchor and a far-away code. Its width covers real
//     prose such as "status: Internal Server Error, transaction 502 aborted".
//   - The code must be preceded by a non-alphanumeric separator and be
//     word-bounded, so "5000ms" and "http5000" never read as a status. A "/"
//     separator is excluded: a code reached through a slash is a URL path
//     segment ("GET https://host/orders/500"), not a status.
//   - Group 1 captures a trailing duration unit, if any, so a code that is
//     really a duration ("response time was 503 ms") can be rejected in
//     matches5xx.
var re5xx = regexp.MustCompile(
	`\b(?:status|http|response|responded|returned|code\b)` + // anchor word
		`[^0-9\n]{0,40}[^0-9A-Za-z/\n]` + // digit-free gap, then a non-slash separator
		`5\d\d\b` + // the status code itself
		`(?:\s?(ms|msec|msecs|millis|milliseconds|s|sec|secs|second|seconds|m|min|mins|minute|minutes|h|hr|hrs|hour|hours)\b)?`)

// matches5xx reports whether s mentions a 5xx status code. Matches whose code
// is immediately followed by a duration unit are skipped: those are timings
// that happen to fall in the 500-599 range, not status codes.
func matches5xx(s string) bool {
	for _, m := range re5xx.FindAllStringSubmatch(s, -1) {
		if m[1] == "" {
			return true
		}
	}
	return false
}

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
	// The camelCase spellings Playwright, Jest and Mocha actually print. The
	// spaced variants above never match a real "beforeEach" frame.
	"beforeeach",
	"aftereach",
	"beforeall",
	"afterall",
	// Java/TestNG annotations, kept in before/after pairs: a teardown hook is
	// as much a test bug as its setup twin.
	"@beforeclass",
	"@afterclass",
	"@beforemethod",
	"@aftermethod",
	"@beforesuite",
	"@aftersuite",
	"conftest",
	"setup_method",
	"teardown_method",
	"setup_class",
	"teardown_class",
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

	// A 5xx reaching this point was not phrased as an assertion, so it is a
	// dependency or environment failing under the test rather than the product
	// asserting wrongly. The ordering is deliberate: "Expected status 200,
	// received 503" is matched by productBugKeywords above and stays a product
	// bug, because a correct expectation met by a server error is the product
	// misbehaving. Only non-assertion 5xx prose reaches here.
	if matches5xx(combined) {
		return store.DefectCategoryInfrastructure
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
