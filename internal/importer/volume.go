package importer

import (
	"regexp"
	"strings"

	"github.com/vavallee/bindery/internal/seriesmatch"
)

// partMarkerRe matches a "Part N" marker, with an optional "of M": "Part 1",
// "Pt. 2", "Part 1 of 3".
var partMarkerRe = regexp.MustCompile(`(?i)\b(?:part|pt)\.?\s*\d+(?:\.\d+)?(?:\s+of\s+\d+)?\b`)

// volumeTitles returns a and b ready for a volume-number comparison.
//
// seriesmatch.VolumeNumber counts "Part N" as a volume marker, which is right
// when both sides say "Part": "The Way of Kings, Part 1" and "Part 2" are the
// two halves of a split edition, and different books. Beside a library file it
// is usually something else. A multi-file audiobook is delivered as "Title
// Part 1.mp3", "Title Part 2.mp3" or as Part 1/ and Part 2/ folders, and there
// the number counts files, not books. Compared against a wanted "Rhythm of War
// (The Stormlight Archive, Book 4)", the "Part 1" read as volume 1 and vetoed
// the book's own files (#2810 review). So a Part marker on one side only is
// dropped before the comparison, and kept when both sides carry one.
func volumeTitles(a, b string) (string, string) {
	ap, bp := partMarkerRe.MatchString(a), partMarkerRe.MatchString(b)
	switch {
	case ap && !bp:
		a = stripPartMarker(a)
	case bp && !ap:
		b = stripPartMarker(b)
	}
	return a, b
}

func stripPartMarker(s string) string {
	s = partMarkerRe.ReplaceAllString(s, " ")
	return strings.TrimRight(strings.TrimSpace(s), " ,.:;-_")
}

// differentVolumes is seriesmatch.DifferentVolumes after volumeTitles: true
// only when both titles carry a volume number and the numbers disagree.
func differentVolumes(a, b string) bool {
	a, b = volumeTitles(a, b)
	return seriesmatch.DifferentVolumes(a, b)
}

// trailingDigitRe reports a title that ends in a number, the bare-number
// spelling seriesmatch.DifferentVolumes compares ("Defiance of the Fall 01").
var trailingDigitRe = regexp.MustCompile(`\d\s*$`)

// carriesVolumeNumber reports whether s has a number DifferentVolumes could
// compare: an explicit marker ("Vol. 3", "Book 3", "#3") or a trailing one.
func carriesVolumeNumber(s string) bool {
	if _, ok := seriesmatch.VolumeNumber(s); ok {
		return true
	}
	return trailingDigitRe.MatchString(s)
}
