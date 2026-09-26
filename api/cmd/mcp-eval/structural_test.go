package main

import (
	"strings"
	"testing"
)

func TestValidateDiagnoseStructure(t *testing.T) {
	const valid = `{"build":{"build_id":1},"failing_tests":[{"full_name":"a.Test1"}],"examined_tests":1}`
	tests := []struct {
		name       string
		raw        string
		maxBytes   int
		wantIssues []string // substrings expected in the issues, in order
	}{
		{"valid payload passes", valid, 100000, nil},
		{"invalid JSON", `{not json`, 100000, []string{"invalid JSON"}},
		{"missing build key", `{"failing_tests":[{"full_name":"a.Test1"}],"examined_tests":1}`, 100000,
			[]string{`missing "build" key`}},
		{"null build value", `{"build":null,"failing_tests":[{"full_name":"a.Test1"}],"examined_tests":1}`, 100000,
			[]string{`missing "build" key`}},
		{"missing failing_tests key", `{"build":{"build_id":1},"examined_tests":1}`, 100000,
			[]string{`missing or non-array "failing_tests" key`, `examined_tests=1 does not match len(failing_tests)=0`}},
		{"failing_tests not an array", `{"build":{"build_id":1},"failing_tests":"oops","examined_tests":0}`, 100000,
			[]string{`missing or non-array "failing_tests" key`}},
		// Regression guard for the dedup bug where duplicate rows doubled every payload.
		{"duplicated full_name payload must fail", `{"build":{"build_id":1},"failing_tests":[{"full_name":"a.Test1"},{"full_name":"a.Test1"},{"full_name":"a.Test2"}],"examined_tests":3}`, 100000,
			[]string{"duplicate full_name in failing_tests: [a.Test1]"}},
		{"multiple duplicates sorted", `{"build":{"build_id":1},"failing_tests":[{"full_name":"b.Test"},{"full_name":"a.Test"},{"full_name":"b.Test"},{"full_name":"a.Test"}],"examined_tests":4}`, 100000,
			[]string{"duplicate full_name in failing_tests: [a.Test b.Test]"}},
		{"entries without a full_name are not duplicates", `{"build":{"build_id":1},"failing_tests":[{"status":"failed"},{"status":"failed"},"not an object"],"examined_tests":3}`, 100000,
			nil},
		{"examined_tests missing", `{"build":{"build_id":1},"failing_tests":[{"full_name":"a.Test1"}]}`, 100000,
			[]string{`missing "examined_tests" key`}},
		{"examined_tests wrong type", `{"build":{"build_id":1},"failing_tests":[{"full_name":"a.Test1"}],"examined_tests":"one"}`, 100000,
			[]string{`"examined_tests" is not a number`}},
		{"examined_tests mismatch", `{"build":{"build_id":1},"failing_tests":[{"full_name":"a.Test1"}],"examined_tests":5}`, 100000,
			[]string{"examined_tests=5 does not match len(failing_tests)=1"}},
		{"byte size exceeds ceiling", valid, 10, []string{"exceeds max_diagnose_bytes 10"}},
		{"maxBytes of zero disables the size check", valid, 0, nil},
		{"failing_tests within the build's failed+broken stats passes", `{"build":{"build_id":1,"total_tests":10,"failed_tests":1,"broken_tests":1},"failing_tests":[{"full_name":"a.Test1"},{"full_name":"a.Test2"}],"examined_tests":2}`, 100000,
			nil},
		{"failing_tests exceeding the build's failed+broken stats fails", `{"build":{"build_id":1,"total_tests":10,"failed_tests":1,"broken_tests":0},"failing_tests":[{"full_name":"a.Test1"},{"full_name":"a.Test2"}],"examined_tests":2}`, 100000,
			[]string{"len(failing_tests)=2 exceeds build failed_tests+broken_tests=1"}},
		{"zero total_tests skips the stats check", `{"build":{"build_id":1,"total_tests":0,"failed_tests":0,"broken_tests":0},"failing_tests":[{"full_name":"a.Test1"},{"full_name":"a.Test2"}],"examined_tests":2}`, 100000,
			nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateDiagnoseStructure([]byte(tc.raw), tc.maxBytes)
			if len(got) != len(tc.wantIssues) {
				t.Fatalf("validateDiagnoseStructure() = %v, want %d issue(s) containing %v", got, len(tc.wantIssues), tc.wantIssues)
			}
			for i, want := range tc.wantIssues {
				if !strings.Contains(got[i], want) {
					t.Errorf("issue[%d] = %q, want substring %q", i, got[i], want)
				}
			}
		})
	}
}
