package parity

import (
	"strings"
	"testing"
)

// multipartFixture builds a body from (name, filename, content) triples so a
// test can state a part order directly.
func multipartFixture(parts ...[3]string) string {
	var body strings.Builder
	for _, part := range parts {
		body.WriteString("--" + multipartFixedBoundary + "\r\n")
		if part[1] == "" {
			body.WriteString(`Content-Disposition: form-data; name="` + part[0] + `"`)
		} else {
			body.WriteString(`Content-Disposition: form-data; name="` + part[0] +
				`"; filename="` + part[1] + `"`)
		}
		body.WriteString("\r\n\r\n" + part[2] + "\r\n")
	}
	body.WriteString("--" + multipartFixedBoundary + "--\r\n")

	return body.String()
}

// TestMultipartComparisonIsOrderSensitive checks the checker.
//
// Multipart parts used to be indexed into a map by field name, so part order
// was invisible and repeated field names collapsed onto one another. Both of
// those made the comparison quietly weaker than it read, and neither showed up
// as a failure anywhere — a checker that cannot reject anything passes every
// fixture. These are the cases it must now reject.
func TestMultipartComparisonIsOrderSensitive(t *testing.T) {
	tests := []struct {
		name    string
		python  string
		go_     string
		wantErr string
	}{
		{
			name:   "identical bodies match",
			python: multipartFixture([3]string{"token", "", "t"}, [3]string{"message", "", "m"}),
			go_:    multipartFixture([3]string{"token", "", "t"}, [3]string{"message", "", "m"}),
		},
		{
			name: "reordered fields are rejected",
			// This is the shape the port used to emit: same fields, same
			// values, sorted rather than in upstream's declared order.
			python:  multipartFixture([3]string{"token", "", "t"}, [3]string{"message", "", "m"}),
			go_:     multipartFixture([3]string{"message", "", "m"}, [3]string{"token", "", "t"}),
			wantErr: "header mismatch",
		},
		{
			name: "a dropped repeat is rejected",
			// Two files under one field name, which the map-based comparison
			// reduced to whichever came last.
			python: multipartFixture(
				[3]string{"file", "a.txt", "aaa"}, [3]string{"file", "b.txt", "bbb"}),
			go_:     multipartFixture([3]string{"file", "b.txt", "bbb"}),
			wantErr: "part count mismatch",
		},
		{
			name: "differing content under a repeated name is rejected",
			python: multipartFixture(
				[3]string{"file", "a.txt", "aaa"}, [3]string{"file", "b.txt", "bbb"}),
			go_: multipartFixture(
				[3]string{"file", "a.txt", "aaa"}, [3]string{"file", "b.txt", "WRONG"}),
			wantErr: "content mismatch",
		},
		{
			name: "json parts still compare structurally",
			python: multipartFixture(
				[3]string{"payload_json", "", `{"a": 1, "b": 2}`}),
			go_: multipartFixture(
				[3]string{"payload_json", "", `{"b":2,"a":1}`}),
		},
		{
			name:    "json parts with different values are rejected",
			python:  multipartFixture([3]string{"payload_json", "", `{"a": 1}`}),
			go_:     multipartFixture([3]string{"payload_json", "", `{"a": 2}`}),
			wantErr: "json content mismatch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := compareMultipartBodies(tc.python, tc.go_)

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected a match, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected a mismatch, the comparison accepted it")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected an error mentioning %q, got: %v", tc.wantErr, err)
			}
		})
	}
}
