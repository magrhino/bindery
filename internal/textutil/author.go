// Package textutil contains small normalization and cleanup helpers used across imports and API responses.
package textutil

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func NormalizeAuthorName(name string) string {
	name = norm.NFC.String(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(name))
	spacePending := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if spacePending && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(unicode.ToLower(r))
			spacePending = false
		default:
			spacePending = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
