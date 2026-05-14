package calibre

import "testing"

func TestNormalizeLanguageForCalibre(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"eng", "en"},
		{"fre", "fr"},
		{"fra", "fr"},
		{"ger", "de"},
		{"deu", "de"},
		{"EN", "en"},
		{"  spa  ", "es"},
		{"cat", "cat"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeLanguageForCalibre(tt.in); got != tt.want {
			t.Errorf("NormalizeLanguageForCalibre(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
