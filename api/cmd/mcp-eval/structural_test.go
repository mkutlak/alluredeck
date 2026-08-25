package main

import (
	"strings"
	"testing"
)

func TestValidateDiagnoseStructure(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		maxBytes   int
		wantIssues []string // substrings expected in the issues, in order
	}{
		{
			name: "valid payload passes",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [
					{"full_name": "a.Test1"},
					{"full_name": "a.Test2"}
				],
				"examined_tests": 2
			}`,
			maxBytes:   100000,
			wantIssues: nil,
		},
		{
			name:       "invalid JSON",
			raw:        `{not json`,
			maxBytes:   100000,
			wantIssues: []string{"invalid JSON"},
		},
		{
			name: "missing build key",
			raw: `{
				"failing_tests": [{"full_name": "a.Test1"}],
				"examined_tests": 1
			}`,
			maxBytes:   100000,
			wantIssues: []string{`missing "build" key`},
		},
		{
			name: "null build value",
			raw: `{
				"build": null,
				"failing_tests": [{"full_name": "a.Test1"}],
				"examined_tests": 1
			}`,
			maxBytes:   100000,
			wantIssues: []string{`missing "build" key`},
		},
		{
			name: "missing failing_tests key",
			raw: `{
				"build": {"build_id": 1},
				"examined_tests": 1
			}`,
			maxBytes:   100000,
			wantIssues: []string{`missing or non-array "failing_tests" key`, `examined_tests=1 does not match len(failing_tests)=0`},
		},
		{
			name: "failing_tests not an array",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": "oops",
				"examined_tests": 0
			}`,
			maxBytes:   100000,
			wantIssues: []string{`missing or non-array "failing_tests" key`},
		},
		{
			name: "duplicated full_name payload must fail",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [
					{"full_name": "a.Test1"},
					{"full_name": "a.Test1"},
					{"full_name": "a.Test2"}
				],
				"examined_tests": 3
			}`,
			maxBytes:   100000,
			wantIssues: []string{"duplicate full_name in failing_tests: [a.Test1]"},
		},
		{
			name: "multiple duplicates sorted",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [
					{"full_name": "b.Test"},
					{"full_name": "a.Test"},
					{"full_name": "b.Test"},
					{"full_name": "a.Test"}
				],
				"examined_tests": 4
			}`,
			maxBytes:   100000,
			wantIssues: []string{"duplicate full_name in failing_tests: [a.Test b.Test]"},
		},
		{
			name: "examined_tests missing",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [{"full_name": "a.Test1"}]
			}`,
			maxBytes:   100000,
			wantIssues: []string{`missing "examined_tests" key`},
		},
		{
			name: "examined_tests wrong type",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [{"full_name": "a.Test1"}],
				"examined_tests": "one"
			}`,
			maxBytes:   100000,
			wantIssues: []string{`"examined_tests" is not a number`},
		},
		{
			name: "examined_tests mismatch",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [{"full_name": "a.Test1"}],
				"examined_tests": 5
			}`,
			maxBytes:   100000,
			wantIssues: []string{"examined_tests=5 does not match len(failing_tests)=1"},
		},
		{
			name: "byte size exceeds ceiling",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [{"full_name": "a.Test1"}],
				"examined_tests": 1
			}`,
			maxBytes:   10,
			wantIssues: []string{"exceeds max_diagnose_bytes 10"},
		},
		{
			name: "maxBytes of zero disables the size check",
			raw: `{
				"build": {"build_id": 1},
				"failing_tests": [{"full_name": "a.Test1"}],
				"examined_tests": 1
			}`,
			maxBytes:   0,
			wantIssues: nil,
		},
		{
			name: "failing_tests within the build's failed+broken stats passes",
			raw: `{
				"build": {"build_id": 1, "total_tests": 10, "failed_tests": 1, "broken_tests": 1},
				"failing_tests": [{"full_name": "a.Test1"}, {"full_name": "a.Test2"}],
				"examined_tests": 2
			}`,
			maxBytes:   100000,
			wantIssues: nil,
		},
		{
			name: "failing_tests exceeding the build's failed+broken stats fails",
			raw: `{
				"build": {"build_id": 1, "total_tests": 10, "failed_tests": 1, "broken_tests": 0},
				"failing_tests": [{"full_name": "a.Test1"}, {"full_name": "a.Test2"}],
				"examined_tests": 2
			}`,
			maxBytes:   100000,
			wantIssues: []string{"len(failing_tests)=2 exceeds build failed_tests+broken_tests=1"},
		},
		{
			name: "zero total_tests skips the stats check",
			raw: `{
				"build": {"build_id": 1, "total_tests": 0, "failed_tests": 0, "broken_tests": 0},
				"failing_tests": [{"full_name": "a.Test1"}, {"full_name": "a.Test2"}],
				"examined_tests": 2
			}`,
			maxBytes:   100000,
			wantIssues: nil,
		},
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

func TestDuplicateFullNames(t *testing.T) {
	tests := []struct {
		name         string
		failingTests []any
		want         []string
	}{
		{name: "empty", failingTests: nil, want: nil},
		{
			name: "no duplicates",
			failingTests: []any{
				map[string]any{"full_name": "a.Test1"},
				map[string]any{"full_name": "a.Test2"},
			},
			want: nil,
		},
		{
			name: "one duplicate pair",
			failingTests: []any{
				map[string]any{"full_name": "a.Test1"},
				map[string]any{"full_name": "a.Test1"},
			},
			want: []string{"a.Test1"},
		},
		{
			name: "entries missing full_name are ignored",
			failingTests: []any{
				map[string]any{"status": "failed"},
				map[string]any{"status": "failed"},
			},
			want: nil,
		},
		{
			name: "non-object entries are ignored",
			failingTests: []any{
				"not an object",
				map[string]any{"full_name": "a.Test1"},
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := duplicateFullNames(tc.failingTests)
			if len(got) != len(tc.want) {
				t.Fatalf("duplicateFullNames() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("duplicateFullNames() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestExaminedTestsMismatch(t *testing.T) {
	tests := []struct {
		name         string
		doc          map[string]any
		failingTests []any
		wantEmpty    bool
	}{
		{
			name:         "matches",
			doc:          map[string]any{"examined_tests": float64(2)},
			failingTests: []any{map[string]any{}, map[string]any{}},
			wantEmpty:    true,
		},
		{
			name:         "missing key",
			doc:          map[string]any{},
			failingTests: nil,
			wantEmpty:    false,
		},
		{
			name:         "wrong type",
			doc:          map[string]any{"examined_tests": "2"},
			failingTests: nil,
			wantEmpty:    false,
		},
		{
			name:         "mismatch",
			doc:          map[string]any{"examined_tests": float64(3)},
			failingTests: []any{map[string]any{}},
			wantEmpty:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := examinedTestsMismatch(tc.doc, tc.failingTests)
			if (got == "") != tc.wantEmpty {
				t.Fatalf("examinedTestsMismatch() = %q, wantEmpty=%v", got, tc.wantEmpty)
			}
		})
	}
}
