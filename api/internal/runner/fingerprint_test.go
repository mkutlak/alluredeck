package runner

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

const (
	infra       = store.DefectCategoryInfrastructure
	testBug     = store.DefectCategoryTestBug
	productBug  = store.DefectCategoryProductBug
	investigate = store.DefectCategoryToInvestigate
)

// TestNormalizeMessage pins NormalizeMessage's output byte-for-byte. Defect
// fingerprints are SHA-256 digests of this string and are PERSISTED in
// defect_fingerprints.fingerprint_hash, so any change here silently orphans
// every stored fingerprint and re-splits existing defect clusters. Widening
// CategorizeError's keyword set must not disturb these outputs.
func TestNormalizeMessage(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"", ""},
		{"connection refused: 192.168.1.100", "connection refused: <IP>"},
		{"from 10.0.0.1 to 172.16.0.5", "from <IP> to <IP>"},
		{"object 550e8400-e29b-41d4-a716-446655440000 not found", "object <UUID> not found"},
		{"event at 2024-01-15T13:45:00Z failed", "event at <TIMESTAMP> failed"},
		{"time=2023-12-31T23:59:59+05:00", "time=<TIMESTAMP>"},
		{"request failed at 1700000000", "request failed at <TIMESTAMP>"}, // unix seconds
		{"ts=1700000000000 error", "ts=<TIMESTAMP> error"},                // unix millis
		{"error on line 42", "error on line 42"},
		{"port 8080 in use", "port 8080 in use"},
		{"address 0xdeadbeef1234 is invalid", "address <HEX> is invalid"},
		{"code 0x1234 returned", "code 0x1234 returned"}, // < 6 hex digits
		{"failed at /usr/local/bin/myapp", "failed at myapp"},
		{"open /var/lib/data/config/app.conf: no such file", "open app.conf: no such file"},
		{"record 12345 not found", "record <ID> not found"},
		{"transaction 1234567890 failed", "transaction <TIMESTAMP> failed"},
		{strings.Repeat("a", 1100), strings.Repeat("a", 1000)}, // truncated at 1000
		{strings.Repeat("b", 1000), strings.Repeat("b", 1000)},
		{"Error: API call failed with status 500. URL: https://qa.example.com/api/TokenAuth/Authenticate", "Error: API call failed with status 500. URL: https:/Authenticate"},
		{"Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')", "Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')"},
		{"connection refused to 10.0.0.5:8080 at 2026-01-02T03:04:05Z", "connection refused to <IP>:8080 at <TIMESTAMP>"},
		{"beforeEach hook failed: API call failed with status 502", "beforeEach hook failed: API call failed with status 502"},
	}
	for _, tc := range tests {
		if got := NormalizeMessage(tc.in); got != tc.want {
			t.Errorf("NormalizeMessage(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
		}
	}
}

// TestNormalizeTrace covers line-count capping, line number replacement, and path stripping.
func TestNormalizeTrace(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"", ""},
		{"panic: runtime error", "panic: runtime error"},
		{"goroutine 1 [running]:\nmain.main()\n\t/home/user/project/main.go:42 +0x1234", "goroutine 1 [running]:\nmain.main()\n\tmain.go:<LINE> +0x1234"},
		{"main.go:42:10: undefined", "main.go:<LINE>: undefined"},
		{"line1\nline2\nline3\nline4\nline5\nline6\nline7", "line1\nline2\nline3\nline4\nline5"},
		{"line1\nline2\nline3\nline4\nline5", "line1\nline2\nline3\nline4\nline5"},
		{"at /usr/local/lib/myapp/runner.go:99", "at runner.go:<LINE>"},
	}
	for _, tc := range tests {
		if got := NormalizeTrace(tc.in); got != tc.want {
			t.Errorf("NormalizeTrace(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
		}
	}
}

// TestComputeFingerprint pins the persisted hash format: lowercase hex
// SHA-256 of normalizedMsg + "\n" + normalizedTrace.
func TestComputeFingerprint(t *testing.T) {
	t.Parallel()
	const want = "9c01865f0e6d2dd0012fe514a22e065f95fb739f96d05edaa0334dc92253673d"
	if got := ComputeFingerprint("msg", "trace"); got != want {
		t.Errorf("ComputeFingerprint(msg, trace) = %q, want %q", got, want)
	}
}

// TestCategorizeError covers every keyword bucket, case-insensitive matching,
// the camelCase hook names Playwright/Jest/Mocha print and the before/after
// hook pairs, and the bare-5xx fallback with its anchoring rules. Real
// messages from a live report — a TokenAuth 500 in beforeEach (infrastructure)
// and an auto-retrying toast assertion (not infrastructure) — are included.
//
// 5xx precedence: "Expected status 200, received 503" stays a product bug. The
// server answered a correct expectation with the wrong status, which is the
// product misbehaving; productBugKeywords running before the 5xx rule is what
// produces that verdict and is deliberate. The anchoring rows are mirrored in
// triage's TestStatusCodeAgreesWithFingerprintCategory — keep them in sync.
func TestCategorizeError(t *testing.T) {
	t.Parallel()
	tests := []struct{ msg, trace, want string }{
		{"dial tcp: connection refused", "", infra},
		{"connection timed out after 30s", "", infra},
		{"read: connection reset by peer", "", infra},
		{"dns resolution failed for host", "", infra},
		{"dial: no such host", "", infra},
		{"fatal: out of memory", "", infra},
		{"process oom killed by kernel", "", infra},
		{"write failed: disk full", "", infra},
		{"no space left on device", "", infra},
		{"open /etc/secret: permission denied", "", infra},
		{"socket hang up", "", infra},
		{"ECONNREFUSED", "", infra},
		{"ETIMEDOUT", "", infra},
		{"database is locked", "", infra},
		{"too many connections", "", infra},
		{"x509: certificate has expired", "", infra},
		{"ssl handshake failed", "", infra},
		{"tls handshake timeout", "", infra},
		{"DIAL TCP: CONNECTION REFUSED", "", infra},

		{"setup failed: database not reachable", "", testBug},
		{"teardown failed: could not close connection", "", testBug},
		{"fixture 'db' not available", "", testBug},
		{"before each hook failed", "", testBug},
		{"after each hook failed", "", testBug},
		{"beforeEach hook failed", "", testBug},
		{"afterEach hook failed", "", testBug},
		{"beforeAll hook failed", "", testBug},
		{"afterAll hook failed", "", testBug},
		{"@BeforeClass setUp() failed", "", testBug},
		{"@AfterClass tearDown() failed", "", testBug},
		{"@BeforeMethod setUp() failed", "", testBug},
		{"@AfterMethod tearDown() failed", "", testBug},
		{"@BeforeSuite failed to start the grid", "", testBug},
		{"@AfterSuite failed to stop the grid", "", testBug},
		{"error in conftest.py", "", testBug},
		{"setup_method raised an exception", "", testBug},
		{"teardown_method raised an exception", "", testBug},
		{"setup_class raised an exception", "", testBug},
		{"teardown_class raised an exception", "", testBug},
		{"NoSuchElementException: unable to locate element", "", testBug},
		{"StaleElementReferenceException", "", testBug},
		{"timeout waiting for element to appear", "", testBug},
		{"wait_for_selector('.btn') timed out", "", testBug},
		{"element not interactable", "", testBug},

		{"AssertionError: values differ", "", productBug},
		{"assert response.status == 200", "", productBug},
		{"expected 200 but got 404", "", productBug},
		{"expect(result).to equal(true)", "", productBug},
		{"expect(x).to be(null)", "", productBug},
		{"assertEquals(expected, actual) failed", "", productBug},
		{"assertThat(value, is(5))", "", productBug},
		{"expect(foo).toBe(bar)", "", productBug},
		{"NullPointerException at line 42", "", productBug},
		{"TypeError: Cannot read properties of undefined", "", productBug},
		{"IndexOutOfBoundsException: index 5", "", productBug},
		{"KeyError: 'username'", "", productBug},
		{"AttributeError: 'NoneType' object has no attribute 'id'", "", productBug},
		{"Error: Timed out 5000ms waiting for expect(locator).toContainText('Saved')", "", productBug},

		// A bare 5xx not phrased as an assertion is a dependency failing under the test.
		{"request failed with status code 500", "", infra},
		{"request failed", "HTTP status 503 returned", infra},
		{"Error: API call failed with status 500. URL: https://qa.example.com/api/TokenAuth/Authenticate, Method: POST", "", infra},
		{"gateway rejected the request, code: 502", "", infra},
		{"response 504 while loading the dashboard", "", infra},
		{"status: Internal Server Error, transaction 502 aborted", "", infra}, // digit-free gap
		{"HTTP 503 Service Unavailable", "", infra},
		{"API call failed with statusCode: 500", "", infra}, // anchors keep a leading boundary only
		{"request rejected, status_code=503", "", infra},    // so glued spellings still resolve
		{"gateway responded with 502", "", infra},
		{"Expected status 200, received 503", "", productBug}, // assertion wins over 5xx
		{"test failed after 500 ms", "", investigate},         // duration, not a status
		{"response time was 503 ms, over the 200ms budget", "", investigate},
		{"failed to decode 512 bytes", "", investigate}, // "code" inside decode is not an anchor
		{"encoded 550 rows", "", investigate},
		{"Timed out 5000ms waiting for locator", "", investigate},
		{"step timed out after 5000ms", "", investigate},
		{"http 5000 requests queued", "", investigate},                                       // code glued to digits
		{"http request 12 of 40 in batch 7 hit 500", "", investigate},                        // digits break the association
		{"GET https://api.example.com/orders/500 did not return the order", "", investigate}, // URL path segment
		{"request failed with status 404", "", investigate},

		{"something weird happened", "", investigate},
		{"", "", investigate},
		{"panic: interface conversion", "", investigate},
	}
	for _, tc := range tests {
		if got := CategorizeError(tc.msg, tc.trace); got != tc.want {
			t.Errorf("CategorizeError(%q, %q)\n  got:  %q\n  want: %q", tc.msg, tc.trace, got, tc.want)
		}
	}
}

// TestComputeFingerprintsForResults checks grouping by normalized signature,
// the effective-message fallback (first trace line, then a placeholder), the
// per-group category, and that each map key is the group's hash.
func TestComputeFingerprintsForResults(t *testing.T) {
	t.Parallel()
	type group struct {
		msg, category string
		ids           []int64
	}
	tests := []struct {
		name string
		in   []store.FailedTestResult
		want []group // sorted by msg
	}{
		{name: "empty input"},
		{
			name: "identical errors group, different ones split",
			in:   []store.FailedTestResult{{ID: 10, StatusMessage: "connection refused"}, {ID: 20, StatusMessage: "connection refused"}, {ID: 30, StatusMessage: "assert failed"}},
			want: []group{{"assert failed", productBug, []int64{30}}, {"connection refused", infra, []int64{10, 20}}},
		},
		{
			name: "empty message falls back to the first trace line",
			in:   []store.FailedTestResult{{ID: 1, StatusTrace: "AssertionError: wrong value\n  at test.go:10"}},
			want: []group{{"AssertionError: wrong value", productBug, []int64{1}}},
		},
		{name: "no message or trace uses a placeholder", in: []store.FailedTestResult{{ID: 1}}, want: []group{{"<no message>", investigate, []int64{1}}}},
	}
	for _, tc := range tests {
		var got []group
		for hash, fp := range ComputeFingerprintsForResults(tc.in) {
			if fp.Hash != hash {
				t.Errorf("%s: FingerprintResult.Hash %q does not match map key %q", tc.name, fp.Hash, hash)
			}
			got = append(got, group{fp.NormalizedMessage, fp.Category, fp.TestResultIDs})
		}
		slices.SortFunc(got, func(a, b group) int { return strings.Compare(a.msg, b.msg) })
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: groups = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
