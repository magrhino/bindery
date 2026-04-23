package textutil

import "testing"

func TestCleanDescriptionStripsHTMLAndPreservesParagraphs(t *testing.T) {
	t.Parallel()

	in := `<p><b>Now a STARZ&reg; Original Series</b></p><p>Locked behind bars&nbsp;for three years.</p><p>Line<br>break</p>`
	want := "Now a STARZ® Original Series\n\nLocked behind bars for three years.\n\nLine break"

	if got := CleanDescription(in); got != want {
		t.Fatalf("CleanDescription() = %q, want %q", got, want)
	}
}

func TestCleanDescriptionPlainTextWhitespace(t *testing.T) {
	t.Parallel()

	in := "  A   plain\n  description &amp; more.  "
	want := "A plain description & more."

	if got := CleanDescription(in); got != want {
		t.Fatalf("CleanDescription() = %q, want %q", got, want)
	}
}
