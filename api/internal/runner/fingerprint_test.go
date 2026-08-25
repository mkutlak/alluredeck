package runner

import (
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestNormalizeMessage covers all substitution rules and edge cases.
func TestNormalizeMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "IPv4 address stripped",
			input: "connection refused: 192.168.1.100",
			want:  "connection refused: <IP>",
		},
		{
			name:  "multiple IPv4 addresses",
			input: "from 10.0.0.1 to 172.16.0.5",
			want:  "from <IP> to <IP>",
		},
		{
			name:  "UUID stripped",
			input: "object 550e8400-e29b-41d4-a716-446655440000 not found",
			want:  "object <UUID> not found",
		},
		{
			name:  "ISO 8601 timestamp stripped",
			input: "event at 2024-01-15T13:45:00Z failed",
			want:  "event at <TIMESTAMP> failed",
		},
		{
			name:  "ISO 8601 timestamp with offset",
			input: "time=2023-12-31T23:59:59+05:00",
			want:  "time=<TIMESTAMP>",
		},
		{
			name:  "Unix timestamp (10 digits) stripped",
			input: "request failed at 1700000000",
			want:  "request failed at <TIMESTAMP>",
		},
		{
			name:  "Unix timestamp (13 digits) stripped",
			input: "ts=1700000000000 error",
			want:  "ts=<TIMESTAMP> error",
		},
		{
			name:  "short number preserved",
			input: "error on line 42",
			want:  "error on line 42",
		},
		{
			name:  "4-digit number preserved",
			input: "port 8080 in use",
			want:  "port 8080 in use",
		},
		{
			name:  "hex address stripped",
			input: "address 0xdeadbeef1234 is invalid",
			want:  "address <HEX> is invalid",
		},
		{
			name:  "short hex preserved (less than 6 hex digits)",
			input: "code 0x1234 returned",
			want:  "code 0x1234 returned",
		},
		{
			name:  "absolute path replaced with basename",
			input: "failed at /usr/local/bin/myapp",
			want:  "failed at myapp",
		},
		{
			name:  "deep absolute path replaced with basename",
			input: "open /var/lib/data/config/app.conf: no such file",
			want:  "open app.conf: no such file",
		},
		{
			name:  "long numeric ID stripped",
			input: "record 12345 not found",
			want:  "record <ID> not found",
		},
		{
			name:  "long numeric ID 10 digits",
			input: "transaction 1234567890 failed",
			want:  "transaction <TIMESTAMP> failed",
		},
		{
			name:  "truncation at 1000 chars",
			input: strings.Repeat("a", 1100),
			want:  strings.Repeat("a", 1000),
		},
		{
			name:  "no truncation at exactly 1000",
			input: strings.Repeat("b", 1000),
			want:  strings.Repeat("b", 1000),
		},
		{
			name:  "no truncation under 1000",
			input: "short message",
			want:  "short message",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeMessage(tc.input)
			if got != tc.want {
				t.Errorf("NormalizeMessage(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeTrace covers line-count capping, line number replacement, and path stripping.
func TestNormalizeTrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "single line no modification",
			input: "panic: runtime error",
			want:  "panic: runtime error",
		},
		{
			name:  "line numbers replaced",
			input: "goroutine 1 [running]:\nmain.main()\n\t/home/user/project/main.go:42 +0x1234",
			want:  "goroutine 1 [running]:\nmain.main()\n\tmain.go:<LINE> +0x1234",
		},
		{
			name:  "line:col replaced",
			input: "main.go:42:10: undefined",
			want:  "main.go:<LINE>: undefined",
		},
		{
			name:  "only first 5 lines kept",
			input: "line1\nline2\nline3\nline4\nline5\nline6\nline7",
			want:  "line1\nline2\nline3\nline4\nline5",
		},
		{
			name:  "exactly 5 lines unchanged",
			input: "line1\nline2\nline3\nline4\nline5",
			want:  "line1\nline2\nline3\nline4\nline5",
		},
		{
			name:  "absolute path replaced with basename",
			input: "at /usr/local/lib/myapp/runner.go:99",
			want:  "at runner.go:<LINE>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeTrace(tc.input)
			if got != tc.want {
				t.Errorf("NormalizeTrace(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestComputeFingerprint checks stability, uniqueness, and output format.
func TestComputeFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("stable output", func(t *testing.T) {
		t.Parallel()
		h1 := ComputeFingerprint("msg", "trace")
		h2 := ComputeFingerprint("msg", "trace")
		if h1 != h2 {
			t.Errorf("ComputeFingerprint is not stable: %q != %q", h1, h2)
		}
	})

	t.Run("64 hex chars", func(t *testing.T) {
		t.Parallel()
		h := ComputeFingerprint("hello", "world")
		if len(h) != 64 {
			t.Errorf("expected 64 hex chars, got %d: %q", len(h), h)
		}
		for _, c := range h {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Errorf("non-hex char %q in fingerprint %q", c, h)
				break
			}
		}
	})

	t.Run("different messages produce different hashes", func(t *testing.T) {
		t.Parallel()
		h1 := ComputeFingerprint("error A", "trace")
		h2 := ComputeFingerprint("error B", "trace")
		if h1 == h2 {
			t.Errorf("expected different hashes, both were %q", h1)
		}
	})

	t.Run("different traces produce different hashes", func(t *testing.T) {
		t.Parallel()
		h1 := ComputeFingerprint("msg", "traceA")
		h2 := ComputeFingerprint("msg", "traceB")
		if h1 == h2 {
			t.Errorf("expected different hashes, both were %q", h1)
		}
	})

	t.Run("empty inputs produce valid hash", func(t *testing.T) {
		t.Parallel()
		h := ComputeFingerprint("", "")
		if len(h) != 64 {
			t.Errorf("expected 64 hex chars for empty inputs, got %d", len(h))
		}
	})
}

// TestCategorizeError verifies all category buckets and the default fallback.
func TestCategorizeError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		message  string
		trace    string
		category string
	}{
		// infrastructure
		{
			name:     "connection refused",
			message:  "dial tcp: connection refused",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "connection timed out",
			message:  "connection timed out after 30s",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "connection reset",
			message:  "read: connection reset by peer",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "dns resolution failed",
			message:  "dns resolution failed for host",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "no such host",
			message:  "dial: no such host",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "out of memory",
			message:  "fatal: out of memory",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "oom killed",
			message:  "process oom killed by kernel",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "disk full",
			message:  "write failed: disk full",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "no space left on device",
			message:  "no space left on device",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "permission denied",
			message:  "open /etc/secret: permission denied",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "socket hang up",
			message:  "socket hang up",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "econnrefused",
			message:  "ECONNREFUSED",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "etimedout",
			message:  "ETIMEDOUT",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "database is locked",
			message:  "database is locked",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "too many connections",
			message:  "too many connections",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "certificate error",
			message:  "x509: certificate has expired",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "ssl error",
			message:  "ssl handshake failed",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "tls handshake",
			message:  "tls handshake timeout",
			category: store.DefectCategoryInfrastructure,
		},
		// test_bug
		{
			name:     "setup failed",
			message:  "setup failed: database not reachable",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "teardown failed",
			message:  "teardown failed: could not close connection",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "fixture",
			message:  "fixture 'db' not available",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "before each",
			message:  "before each hook failed",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "after each",
			message:  "after each hook failed",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "beforeall",
			message:  "beforeall failed",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "afterall",
			message:  "afterall failed",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "BeforeClass annotation",
			message:  "@BeforeClass setUp() failed",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "AfterClass annotation",
			message:  "@AfterClass tearDown() failed",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "BeforeMethod annotation",
			message:  "@BeforeMethod failed",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "conftest",
			message:  "error in conftest.py",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "setup_method",
			message:  "setup_method raised an exception",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "teardown_method",
			message:  "teardown_method raised an exception",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "nosuchelement",
			message:  "NoSuchElementException: unable to locate element",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "stale element reference",
			message:  "StaleElementReferenceException",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "timeout waiting for",
			message:  "timeout waiting for element to appear",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "wait_for_selector",
			message:  "wait_for_selector('.btn') timed out",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "element not interactable",
			message:  "element not interactable",
			category: store.DefectCategoryTestBug,
		},
		// product_bug
		{
			name:     "assertionerror",
			message:  "AssertionError: values differ",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "assert keyword",
			message:  "assert response.status == 200",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "expected",
			message:  "expected 200 but got 404",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "to equal",
			message:  "expect(result).to equal(true)",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "to be",
			message:  "expect(x).to be(null)",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "assertequal",
			message:  "assertEquals(expected, actual) failed",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "assertthat",
			message:  "assertThat(value, is(5))",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "expect(",
			message:  "expect(foo).toBe(bar)",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "nullpointerexception",
			message:  "NullPointerException at line 42",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "typeerror cannot read",
			message:  "TypeError: Cannot read properties of undefined",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "indexoutofboundsexception",
			message:  "IndexOutOfBoundsException: index 5",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "keyerror",
			message:  "KeyError: 'username'",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "attributeerror",
			message:  "AttributeError: 'NoneType' object has no attribute 'id'",
			category: store.DefectCategoryProductBug,
		},
		// A bare 5xx that is not phrased as an assertion reads as a dependency
		// failing under the test, not as the product asserting wrongly.
		{
			name:     "5xx status code in message",
			message:  "request failed with status code 500",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "5xx in trace",
			message:  "request failed",
			trace:    "HTTP status 503 returned",
			category: store.DefectCategoryInfrastructure,
		},
		// to_investigate (default)
		{
			name:     "unknown error",
			message:  "something weird happened",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "empty inputs",
			message:  "",
			trace:    "",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "generic panic",
			message:  "panic: interface conversion",
			category: store.DefectCategoryToInvestigate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CategorizeError(tc.message, tc.trace)
			if got != tc.category {
				t.Errorf("CategorizeError(%q, %q)\n  got:  %q\n  want: %q", tc.message, tc.trace, got, tc.category)
			}
		})
	}
}

// TestComputeFingerprintsForResults checks grouping, fallback message logic, and map output.
func TestComputeFingerprintsForResults(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns empty map", func(t *testing.T) {
		t.Parallel()
		got := ComputeFingerprintsForResults(nil)
		if len(got) != 0 {
			t.Errorf("expected empty map, got %d entries", len(got))
		}
	})

	t.Run("identical errors grouped under same hash", func(t *testing.T) {
		t.Parallel()
		results := []store.FailedTestResult{
			{ID: 1, StatusMessage: "connection refused", StatusTrace: ""},
			{ID: 2, StatusMessage: "connection refused", StatusTrace: ""},
		}
		got := ComputeFingerprintsForResults(results)
		if len(got) != 1 {
			t.Fatalf("expected 1 group, got %d", len(got))
		}
		for _, fp := range got {
			if len(fp.TestResultIDs) != 2 {
				t.Errorf("expected 2 IDs in group, got %d", len(fp.TestResultIDs))
			}
		}
	})

	t.Run("different errors in separate groups", func(t *testing.T) {
		t.Parallel()
		results := []store.FailedTestResult{
			{ID: 1, StatusMessage: "error A", StatusTrace: ""},
			{ID: 2, StatusMessage: "error B", StatusTrace: ""},
		}
		got := ComputeFingerprintsForResults(results)
		if len(got) != 2 {
			t.Errorf("expected 2 groups, got %d", len(got))
		}
	})

	t.Run("empty message uses first trace line as message", func(t *testing.T) {
		t.Parallel()
		results := []store.FailedTestResult{
			{ID: 1, StatusMessage: "", StatusTrace: "AssertionError: wrong value\n  at test.go:10"},
		}
		got := ComputeFingerprintsForResults(results)
		if len(got) != 1 {
			t.Fatalf("expected 1 group, got %d", len(got))
		}
		for _, fp := range got {
			if !strings.Contains(fp.NormalizedMessage, "AssertionError") {
				t.Errorf("expected AssertionError in normalized message, got %q", fp.NormalizedMessage)
			}
		}
	})

	t.Run("both empty uses placeholder", func(t *testing.T) {
		t.Parallel()
		results := []store.FailedTestResult{
			{ID: 1, StatusMessage: "", StatusTrace: ""},
		}
		got := ComputeFingerprintsForResults(results)
		if len(got) != 1 {
			t.Fatalf("expected 1 group, got %d", len(got))
		}
		for _, fp := range got {
			if fp.NormalizedMessage != "<no message>" {
				t.Errorf("expected '<no message>', got %q", fp.NormalizedMessage)
			}
		}
	})

	t.Run("hash field populated and 64 chars", func(t *testing.T) {
		t.Parallel()
		results := []store.FailedTestResult{
			{ID: 1, StatusMessage: "some error", StatusTrace: "trace here"},
		}
		got := ComputeFingerprintsForResults(results)
		for hash, fp := range got {
			if len(hash) != 64 {
				t.Errorf("map key hash expected 64 chars, got %d", len(hash))
			}
			if fp.Hash != hash {
				t.Errorf("FingerprintResult.Hash %q does not match map key %q", fp.Hash, hash)
			}
		}
	})

	t.Run("category is set", func(t *testing.T) {
		t.Parallel()
		results := []store.FailedTestResult{
			{ID: 1, StatusMessage: "connection refused", StatusTrace: ""},
		}
		got := ComputeFingerprintsForResults(results)
		for _, fp := range got {
			if fp.Category != store.DefectCategoryInfrastructure {
				t.Errorf("expected infrastructure category, got %q", fp.Category)
			}
		}
	})

	t.Run("test result IDs collected correctly", func(t *testing.T) {
		t.Parallel()
		results := []store.FailedTestResult{
			{ID: 10, StatusMessage: "assert failed", StatusTrace: ""},
			{ID: 20, StatusMessage: "assert failed", StatusTrace: ""},
			{ID: 30, StatusMessage: "assert failed", StatusTrace: ""},
		}
		got := ComputeFingerprintsForResults(results)
		if len(got) != 1 {
			t.Fatalf("expected 1 group, got %d", len(got))
		}
		for _, fp := range got {
			ids := make(map[int64]bool)
			for _, id := range fp.TestResultIDs {
				ids[id] = true
			}
			for _, r := range results {
				if !ids[r.ID] {
					t.Errorf("expected ID %d in group, not found", r.ID)
				}
			}
		}
	})
}

// TestNormalizeMessage_ByteStable pins NormalizeMessage's output byte-for-byte.
// Defect fingerprints are SHA-256 digests of this string and are PERSISTED in
// defect_fingerprints.fingerprint_hash, so any change here silently orphans
// every stored fingerprint and re-splits existing defect clusters. Widening
// CategorizeError's keyword set must not disturb these outputs.
func TestNormalizeMessage_ByteStable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "token auth 500 keeps the status digits and collapses the url path",
			in:   "Error: API call failed with status 500. URL: https://qa.example.com/api/TokenAuth/Authenticate",
			want: "Error: API call failed with status 500. URL: https:/Authenticate",
		},
		{
			name: "playwright expect timeout is left untouched",
			in:   "Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')",
			want: "Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')",
		},
		{
			name: "ip and iso timestamp are substituted",
			in:   "connection refused to 10.0.0.5:8080 at 2026-01-02T03:04:05Z",
			want: "connection refused to <IP>:8080 at <TIMESTAMP>",
		},
		{
			name: "beforeEach camelCase survives normalization",
			in:   "beforeEach hook failed: API call failed with status 502",
			want: "beforeEach hook failed: API call failed with status 502",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeMessage(tc.in); got != tc.want {
				t.Errorf("NormalizeMessage(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCategorizeError_Brittleness covers the widened CategorizeError rules: the
// camelCase Playwright/Jest hook names, and a bare 5xx code sitting next to a
// failure word. Both real messages below come from a live report where four
// tests failed on a TokenAuth 500 raised in beforeEach and three more failed on
// an auto-retrying toast assertion; the first must read as infrastructure and
// the second must not.
func TestCategorizeError_Brittleness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		message  string
		trace    string
		category string
	}{
		{
			name:     "real TokenAuth 500 is infrastructure",
			message:  "Error: API call failed with status 500. URL: https://qa.example.com/api/TokenAuth/Authenticate, Method: POST",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "real toContainText timeout is not infrastructure",
			message:  "Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "5000ms duration is not read as a 5xx code",
			message:  "step timed out after 5000ms",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "bare code colon 502",
			message:  "gateway rejected the request, code: 502",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "bare HTTP 503",
			message:  "upstream returned HTTP 503",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "response with 504",
			message:  "response 504 while loading the dashboard",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "camelCase beforeEach is a test bug",
			message:  "beforeEach hook timed out",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "camelCase beforeAll is a test bug",
			message:  "Error in beforeAll: fixture unavailable",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "camelCase afterEach is a test bug",
			message:  "afterEach cleanup threw",
			category: store.DefectCategoryTestBug,
		},
		{
			name:     "uppercase infra keyword still matches",
			message:  "DIAL TCP: CONNECTION REFUSED",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "mixed case product keyword still matches",
			message:  "AssertionError: Expected 200",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "assertion mentioning a 500 stays a product bug",
			message:  "expected status 200 but received status 500",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "4xx is not a 5xx",
			message:  "request failed with status 404",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "5xx detected in the trace",
			message:  "request failed",
			trace:    "HTTP status 503 returned",
			category: store.DefectCategoryInfrastructure,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CategorizeError(tc.message, tc.trace); got != tc.category {
				t.Errorf("CategorizeError(%q, %q)\n  got:  %q\n  want: %q", tc.message, tc.trace, got, tc.category)
			}
		})
	}
}

// TestCategorizeError_FiveXXAnchoring pins the anchoring rules of the bare-5xx
// fallback. Every message below was hand-checked against the rule: the first
// four are numbers that merely sit near a failure word and must NOT read as
// infrastructure, the rest exercise the anchor set, the assertion-precedence
// decision, and the word boundary on the code itself.
//
// Precedence note: "Expected status 200, received 503" stays a product bug.
// The server answered a correct expectation with the wrong status, which is the
// product misbehaving; productBugKeywords running before the 5xx rule is what
// produces that verdict and is deliberate, not an accident of ordering.
func TestCategorizeError_FiveXXAnchoring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		message  string
		trace    string
		category string
	}{
		{
			name:     "duration next to a failure word is not a status",
			message:  "test failed after 500 ms",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "latency in ms next to response is not a status",
			message:  "response time was 503 ms, over the 200ms budget",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "code inside decode is not an anchor",
			message:  "failed to decode 512 bytes",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "code inside encoded is not an anchor",
			message:  "encoded 550 rows",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "status prose with a distant code is infrastructure",
			message:  "status: Internal Server Error, transaction 502 aborted",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "assertion phrased 5xx stays a product bug",
			message:  "Expected status 200, received 503",
			category: store.DefectCategoryProductBug,
		},
		{
			name:     "playwright locator timeout is not infrastructure",
			message:  "Timed out 5000ms waiting for locator",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "bare HTTP 503 is infrastructure",
			message:  "HTTP 503 Service Unavailable",
			category: store.DefectCategoryInfrastructure,
		},
		// The anchor set keeps a leading word boundary only, so the glued
		// spellings real clients print still resolve.
		{
			name:     "camelCase statusCode still anchors",
			message:  "API call failed with statusCode: 500",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "snake_case status_code still anchors",
			message:  "request rejected, status_code=503",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "responded anchor",
			message:  "gateway responded with 502",
			category: store.DefectCategoryInfrastructure,
		},
		{
			name:     "code glued to digits is not a status",
			message:  "http 5000 requests queued",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "digits between anchor and code break the association",
			message:  "http request 12 of 40 in batch 7 hit 500",
			category: store.DefectCategoryToInvestigate,
		},
		{
			name:     "numeric URL path segment is not a status",
			message:  "GET https://api.example.com/orders/500 did not return the order",
			category: store.DefectCategoryToInvestigate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CategorizeError(tc.message, tc.trace); got != tc.category {
				t.Errorf("CategorizeError(%q, %q)\n  got:  %q\n  want: %q", tc.message, tc.trace, got, tc.category)
			}
		})
	}
}

// TestCategorizeError_HookNameCoverage closes the asymmetry in testBugKeywords:
// the before-side spellings were listed without their after-side twins, and the
// suite-level and class-level hooks were missing entirely.
func TestCategorizeError_HookNameCoverage(t *testing.T) {
	t.Parallel()

	messages := []string{
		"@BeforeMethod setUp() failed",
		"@AfterMethod tearDown() failed",
		"@BeforeSuite failed to start the grid",
		"@AfterSuite failed to stop the grid",
		"@BeforeClass setUp() failed",
		"@AfterClass tearDown() failed",
		"setup_class raised an exception",
		"teardown_class raised an exception",
		"setup_method raised an exception",
		"teardown_method raised an exception",
		"beforeEach hook failed",
		"afterEach hook failed",
		"beforeAll hook failed",
		"afterAll hook failed",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := CategorizeError(msg, ""); got != store.DefectCategoryTestBug {
				t.Errorf("CategorizeError(%q, \"\")\n  got:  %q\n  want: %q", msg, got, store.DefectCategoryTestBug)
			}
		})
	}
}
