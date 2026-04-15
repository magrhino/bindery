package importer

import (
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestDetectDownloadFormat_Ebook(t *testing.T) {
	cases := []struct {
		name  string
		files []string
	}{
		{"epub only", []string{"/dl/book.epub"}},
		{"mobi only", []string{"/dl/book.mobi"}},
		{"azw3 only", []string{"/dl/book.azw3"}},
		{"pdf only", []string{"/dl/book.pdf"}},
		{"multiple ebooks", []string{"/dl/a.epub", "/dl/b.mobi"}},
		{"empty list", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectDownloadFormat(c.files)
			if got != models.MediaTypeEbook {
				t.Errorf("detectDownloadFormat(%v) = %q, want %q", c.files, got, models.MediaTypeEbook)
			}
		})
	}
}

func TestDetectDownloadFormat_Audiobook(t *testing.T) {
	cases := []struct {
		name  string
		files []string
	}{
		{"m4b only", []string{"/dl/book.m4b"}},
		{"mp3 only", []string{"/dl/chapter01.mp3"}},
		{"m4a only", []string{"/dl/book.m4a"}},
		{"flac only", []string{"/dl/book.flac"}},
		{"opus only", []string{"/dl/book.opus"}},
		{"mixed: audio wins", []string{"/dl/book.epub", "/dl/book.m4b"}},
		{"mp3 chapters + cover", []string{"/dl/ch1.mp3", "/dl/ch2.mp3", "/dl/cover.jpg"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectDownloadFormat(c.files)
			if got != models.MediaTypeAudiobook {
				t.Errorf("detectDownloadFormat(%v) = %q, want %q", c.files, got, models.MediaTypeAudiobook)
			}
		})
	}
}

// TestDetectDownloadFormat_CaseInsensitive checks that uppercase extensions are
// treated the same as lowercase.
func TestDetectDownloadFormat_CaseInsensitive(t *testing.T) {
	if got := detectDownloadFormat([]string{"/dl/BOOK.M4B"}); got != models.MediaTypeAudiobook {
		t.Errorf("expected audiobook for .M4B, got %q", got)
	}
	if got := detectDownloadFormat([]string{"/dl/BOOK.EPUB"}); got != models.MediaTypeEbook {
		t.Errorf("expected ebook for .EPUB, got %q", got)
	}
}
