package indexer

import "regexp"

// Multi-book pack detection for release names (#2276).
//
// A Bindery download row carries exactly one BookID, and the importer
// computes exactly one destination folder from it. A release that is
// explicitly several books therefore has no correct outcome: every file in
// it lands in one book's folder. Nothing downstream can recover from that,
// so the release has to be refused at selection time.
//
// This is deliberately NOT the bundle detection in internal/metadata
// (bundle_titles.go). That one reads OpenLibrary catalogue titles, which are
// editorial prose; these are scene release names, which are punctuation
// soup. Its multi-title branch keys on spaced slashes, and the release that
// reported this issue carries "[ENG / M4B MP3]" — a tag separator, not an
// anthology. Reusing it here would have produced exactly the wrong rejection
// for exactly the release that motivated the work.
//
// What is matched, and why the boundaries sit where they do:
//
//   - An explicit numbered range after "book", "vol" or "volume": "Books 1-4",
//     "Book 1 to 4", "Vols. 1 & 2". Both numbers are capped at two digits so
//     "Book 1 - 2020 Edition" is not read as a range of 2020 books.
//   - A counted bundle: "4 Book Set", "3 Volume Collection".
//   - "box set" / "boxed set" / "boxset", "omnibus", and "complete series" or
//     "complete collection".
//
// What is deliberately NOT matched:
//
//   - "part". A single audiobook split into "Part 1-2" and "Part 3" is the
//     normal shape of a long recording, and is in fact the exact shape of the
//     download in #2275. Treating it as a pack would refuse ordinary
//     audiobooks.
//   - "trilogy". Real single books are routinely subtitled "Book III of the
//     Red Rising Trilogy" — again, the very release under discussion.
//   - A bare "collection", "anthology" or "series". Single books use all
//     three about themselves ("The Collected Stories", "Red Rising Series -
//     Book 1").
//   - A bare numeric range with no preceding noun ("01-04"), which in a
//     release name is far more often a year span, a bitrate, or a chapter
//     count than a book range.
var multiBookPackRe = regexp.MustCompile(`(?i)` +
	`\b(?:books?|vols?|volumes?)\.?\s*\d{1,2}\s*(?:[-–—+&]|to|thru|through)\s*\d{1,2}\b` +
	`|\b\d{1,2}\s*(?:books?|vol(?:ume)?s?)\s+(?:set|collection|bundle|pack|omnibus)\b` +
	`|\bbox(?:ed)?\s*set\b` +
	`|\bomnibus\b` +
	`|\bcomplete\s+(?:series|collection|saga)\b`)

// MultiBookPackMarker returns the substring of title that identifies it as a
// pack of several books, or "" when it names a single book. The matched text
// is returned rather than a bool so callers can tell the user which words
// they are being judged on.
func MultiBookPackMarker(title string) string {
	return multiBookPackRe.FindString(title)
}
