package indexer

import (
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/vavallee/bindery/internal/indexer/newznab"
)

// NormalizeTitleForDedup returns a canonical form of title used as the
// deduplication key when comparing book rows. The normalization is applied
// symmetrically: both when seeding the "already-seen" set from existing DB
// rows and when keying incoming provider results. This guarantees that two
// rows for the same work only differ in edition qualifier, whitespace,
// Unicode form, or umlaut representation are collapsed to the same key.
//
// Steps applied (in order):
//  1. Unicode NFC — composes combining characters into precomposed forms,
//     so "é" (NFD) and "é" (NFC) produce the same key.
//  2. newznab.NormalizeQueryTitle — folds smart quotes to ASCII, strips a
//     trailing parenthesised edition qualifier ("(German Edition)" etc.),
//     and collapses internal whitespace.
//  3. strings.ToLower — case-insensitive match.
//  4. transliterateUmlauts — maps ä→ae, ö→oe, ü→ue, ß→ss so that
//     "Geraeusch" from a release title compares equal to "Geräusch" from
//     the metadata provider.
func NormalizeTitleForDedup(title string) string {
	title = norm.NFC.String(title)
	title = newznab.NormalizeQueryTitle(title)
	title = strings.ToLower(title)
	title = transliterateUmlauts(title)
	return title
}
