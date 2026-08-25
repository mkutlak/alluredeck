package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// defaultMaxDiagnoseBytes is the ceiling applied to a diagnose_failure
// response's marshaled JSON size when a fixture does not set
// max_diagnose_bytes.
const defaultMaxDiagnoseBytes = 100000

// validateDiagnoseStructure runs the structural assertions against a
// diagnose_failure response's marshaled JSON bytes:
//
//   - the document is valid JSON with "build" and "failing_tests" keys
//   - no two failing_tests entries share full_name (dedup regression guard)
//   - examined_tests equals len(failing_tests)
//   - len(failing_tests) does not exceed the build's own failed+broken counts
//   - the byte size is under maxBytes
//
// These are pure shape/size checks, run before any LLM call, so a response
// regression (e.g. duplicate rows silently doubling every payload) fails the
// gate even when the LLM would still have guessed the right category. It
// returns one message per violation found; a nil/empty result means the
// response passed every check.
func validateDiagnoseStructure(raw []byte, maxBytes int) []string {
	var issues []string

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return []string{fmt.Sprintf("invalid JSON: %v", err)}
	}

	if v, ok := doc["build"]; !ok || v == nil {
		issues = append(issues, `missing "build" key`)
	}

	failingTests, hasFailingTests := doc["failing_tests"].([]any)
	if !hasFailingTests {
		issues = append(issues, `missing or non-array "failing_tests" key`)
	}

	if dupes := duplicateFullNames(failingTests); len(dupes) > 0 {
		issues = append(issues, fmt.Sprintf("duplicate full_name in failing_tests: %v", dupes))
	}

	if msg := examinedTestsMismatch(doc, failingTests); msg != "" {
		issues = append(issues, msg)
	}

	if msg := failingTestsExceedBuildStats(doc, failingTests); msg != "" {
		issues = append(issues, msg)
	}

	if maxBytes > 0 && len(raw) > maxBytes {
		issues = append(issues, fmt.Sprintf("response size %d bytes exceeds max_diagnose_bytes %d", len(raw), maxBytes))
	}

	return issues
}

// duplicateFullNames returns the full_name values that appear more than once
// among failingTests, sorted for deterministic output. It is the regression
// guard for the dedup bug where duplicate rows doubled every payload
// unnoticed. Entries without a string full_name are ignored.
func duplicateFullNames(failingTests []any) []string {
	counts := make(map[string]int, len(failingTests))
	for _, t := range failingTests {
		entry, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["full_name"].(string)
		if name == "" {
			continue
		}
		counts[name]++
	}
	var dupes []string
	for name, n := range counts {
		if n > 1 {
			dupes = append(dupes, name)
		}
	}
	sort.Strings(dupes)
	return dupes
}

// failingTestsExceedBuildStats reports a non-empty violation message when the
// response lists more failing tests than the build itself recorded as
// failed+broken.
//
// Unlike examinedTestsMismatch — which compares two fields the same handler
// derives from one another and so can never disagree — this cross-checks the
// per-test array against a count the ingestion pipeline wrote independently.
// A dedup regression that leaks twin rows through therefore fails here even
// when every leaked full_name happens to be unique.
//
// total_tests gates the check because diagnose_failure emits failed_tests and
// broken_tests unconditionally, as 0, for a build whose report carried no
// stats at all. Reading that 0 as "no failures allowed" would fail every such
// legacy build; a zero total is the marker that no counts were recorded.
func failingTestsExceedBuildStats(doc map[string]any, failingTests []any) string {
	build, ok := doc["build"].(map[string]any)
	if !ok {
		return ""
	}
	if total, ok := build["total_tests"].(float64); !ok || total <= 0 {
		return ""
	}
	failed, _ := build["failed_tests"].(float64)
	broken, _ := build["broken_tests"].(float64)
	limit := int(failed) + int(broken)
	if len(failingTests) > limit {
		return fmt.Sprintf("len(failing_tests)=%d exceeds build failed_tests+broken_tests=%d",
			len(failingTests), limit)
	}
	return ""
}

// examinedTestsMismatch reports a non-empty violation message when
// examined_tests is missing, non-numeric, or does not equal
// len(failingTests).
func examinedTestsMismatch(doc map[string]any, failingTests []any) string {
	raw, ok := doc["examined_tests"]
	if !ok {
		return `missing "examined_tests" key`
	}
	examined, ok := raw.(float64)
	if !ok {
		return `"examined_tests" is not a number`
	}
	if int(examined) != len(failingTests) {
		return fmt.Sprintf("examined_tests=%d does not match len(failing_tests)=%d", int(examined), len(failingTests))
	}
	return ""
}
