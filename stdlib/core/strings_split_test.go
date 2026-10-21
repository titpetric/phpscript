package core

import (
	"testing"
)

// TestToString pins the strconv formatting against the fmt formatting it
func TestPHPStrReplace(t *testing.T) {
	t.Parallel()
	cases := []struct {
		search, replace, subject any
		want                     string
	}{
		{"a", "b", "banana", "bbnbnb"},
		{[]string{"a", "n"}, "-", "banana", "b-----"},
		{[]string{"a", "n"}, []string{"1", "2"}, "banana", "b12121"},
		{[]string{"a", "n"}, []string{"1"}, "banana", "b111"},
		{"a", 1, "banana", "b1n1n1"},
	}
	for _, tc := range cases {
		got, err := phpStrReplace(tc.search, tc.replace, tc.subject)
		if err != nil {
			t.Fatalf("str_replace(%v, %v, %v) returned %v", tc.search, tc.replace, tc.subject, err)
		}
		if got != tc.want {
			t.Errorf("str_replace(%v, %v, %v) = %q, want %q", tc.search, tc.replace, tc.subject, got, tc.want)
		}
	}
}
