package models

import (
	"reflect"
	"testing"
)

func TestParseAllowedLanguages(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"any", nil},
		{"ANY", nil},
		{"eng", []string{"eng"}},
		{"eng,fre,ger", []string{"eng", "fre", "ger"}},
		{" Eng , FRE ,  ger ", []string{"eng", "fre", "ger"}},
		{"eng,,fre", []string{"eng", "fre"}},
		// A single "any" anywhere short-circuits to no filter — having a
		// mixed list with "any" in it is contradictory and we treat the
		// broader setting as the user's real intent.
		{"eng,any,fre", nil},
	}
	for _, tc := range cases {
		got := ParseAllowedLanguages(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseAllowedLanguages(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeLanguageCode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"en", "eng"},
		{"EN", "eng"},
		{"en-US", "eng"},
		{"pt_BR", "por"},
		{"zh-Hans", "chi"},
		{"de", "ger"},
		// Already three-letter: passed through lowercased unchanged.
		{"eng", "eng"},
		{"GER", "ger"},
		// Unknown two-letter code round-trips rather than being dropped.
		{"xx", "xx"},
	}
	for _, tc := range cases {
		if got := NormalizeLanguageCode(tc.in); got != tc.want {
			t.Errorf("NormalizeLanguageCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsLanguageAllowed(t *testing.T) {
	cases := []struct {
		code        string
		allowed     []string
		unknownFail bool
		want        bool
	}{
		{"eng", nil, false, true},
		{"eng", nil, true, true},
		{"", []string{"eng"}, false, true},
		{"", []string{"eng"}, true, false},
		{"eng", []string{"eng"}, false, true},
		{"ENG", []string{"eng"}, false, true},
		{" eng ", []string{"eng"}, false, true},
		{"fre", []string{"eng"}, false, false},
		{"fre", []string{"eng", "fre"}, false, true},
		{"ger", []string{"eng", "fre"}, false, false},
		// #1729: provider vocabularies must converge on the profile's. Google
		// Books returns ISO 639-1 ("en", "en-US"); a profile allowing "eng"
		// must accept them.
		{"en", []string{"eng"}, false, true},
		{"en-US", []string{"eng"}, false, true},
		{"EN", []string{"eng"}, false, true},
		{"pt_BR", []string{"por"}, false, true},
		// The allowed side is normalized too, so a profile written in
		// two-letter codes still filters correctly.
		{"eng", []string{"en"}, false, true},
		// A genuinely different language is still rejected.
		{"de", []string{"eng"}, false, false},
		{"fr-CA", []string{"eng"}, false, false},
		// Unknown-language behavior is unchanged by normalization.
		{"", []string{"en"}, false, true},
		{"", []string{"en"}, true, false},
	}
	for _, tc := range cases {
		got := IsLanguageAllowed(tc.code, tc.allowed, tc.unknownFail)
		if got != tc.want {
			t.Errorf("IsLanguageAllowed(%q, %v, unknownFail=%v) = %v, want %v", tc.code, tc.allowed, tc.unknownFail, got, tc.want)
		}
	}
}
