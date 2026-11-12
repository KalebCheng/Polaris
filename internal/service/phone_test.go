package service

import (
	"reflect"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "mainland national", in: "13800000000", want: "+8613800000000"},
		{name: "mainland spaced", in: "138 0000 0000", want: "+8613800000000"},
		{name: "mainland country code", in: "8613800000000", want: "+8613800000000"},
		{name: "mainland plus", in: "+8613800000000", want: "+8613800000000"},
		{name: "mainland 00 prefix", in: "008613800000000", want: "+8613800000000"},
		{name: "international", in: "+1 (415) 555-2671", want: "+14155552671"},
		{name: "unknown country", in: "6591234567", want: "6591234567"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePhone(tc.in)
			if err != nil {
				t.Fatalf("normalizePhone(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizePhone(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizePhoneRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "abc", "+", "123", "1234567890123456", "+86 138 0000 0000 x1"} {
		if _, err := normalizePhone(input); err == nil {
			t.Fatalf("normalizePhone(%q) should fail", input)
		}
	}
}

func TestPhoneLookupCandidates(t *testing.T) {
	got := phoneLookupCandidates("+8613800000000")
	want := []string{"+8613800000000", "8613800000000", "13800000000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("phoneLookupCandidates() = %v, want %v", got, want)
	}
}
