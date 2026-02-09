package main

import "testing"

func TestClassifyE2EFailure(t *testing.T) {
	cases := []struct {
		name    string
	message string
		want    string
	}{
		{
			name:    "timeout",
			message: "timeout waiting for response",
			want:    "infrastructure-flake",
		},
		{
			name:    "scenario",
			message: "missing frame id in stack trace",
			want:    "scenario-precondition",
		},
		{
			name:    "product",
			message: "unexpected setBreakpoints failure",
			want:    "product-defect",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyE2EFailure(tc.message)
			if got != tc.want {
				t.Fatalf("classifyE2EFailure(%q)=%q, want %q", tc.message, got, tc.want)
			}
		})
	}
}
