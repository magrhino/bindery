// Package isbnutil normalizes ISBN inputs for metadata provider lookups.
package isbnutil

import (
	"strings"
	"unicode"
)

// Normalize strips common ISBN separators and uppercases ISBN-10 check digits.
// It intentionally leaves other characters alone so invalid inputs still fail
// at the provider instead of being silently rewritten into a different value.
func Normalize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'x' || r == 'X':
			b.WriteByte('X')
		case r == '-' || r == '_' || unicode.IsSpace(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ToISBN13 normalizes raw and returns it in 13-digit ISBN-13 form, converting
// an ISBN-10 by prefixing "978" and recomputing the EAN-13 check digit. It
// returns "" for anything that is not shaped like an ISBN-10 or ISBN-13, so
// callers can treat "" as "no usable ISBN" rather than having to re-validate.
//
// Bindery needs this because the two sides of an ISBN comparison can be
// recorded in different forms: a release name only ever yields an ISBN-13
// (indexer.ParseRelease requires a 978/979 prefix), while a catalogue edition
// may carry only isbn_10.
func ToISBN13(raw string) string {
	s := Normalize(raw)
	switch len(s) {
	case 13:
		if !allDigits(s) || (!strings.HasPrefix(s, "978") && !strings.HasPrefix(s, "979")) {
			return ""
		}
		return s
	case 10:
		// The ISBN-10 check digit is discarded (it may be 'X', which has no
		// place in an ISBN-13); only the 9-digit registrant core carries over.
		if !allDigits(s[:9]) || (!allDigits(s[9:]) && s[9] != 'X') {
			return ""
		}
		body := "978" + s[:9]
		return body + string('0'+ean13CheckDigit(body))
	default:
		return ""
	}
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ean13CheckDigit computes the trailing check digit for the 12 leading digits
// of an EAN-13/ISBN-13: digits are weighted 1,3,1,3… and the check digit is
// whatever brings the weighted sum to a multiple of 10.
func ean13CheckDigit(body string) byte {
	sum := 0
	for i := 0; i < len(body); i++ {
		d := int(body[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return byte((10 - sum%10) % 10)
}
