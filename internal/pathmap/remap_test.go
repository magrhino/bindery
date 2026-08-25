package pathmap

import (
	"strings"
	"testing"
)

func TestRemapperApplyAndInverse(t *testing.T) {
	r := Parse("/media/downloads:/books/downloads,/media:/books")

	if got := r.Apply("/media/downloads/A"); got != "/books/downloads/A" {
		t.Fatalf("Apply longest prefix = %q, want /books/downloads/A", got)
	}
	if got := r.ApplyInverse("/books/downloads/A"); got != "/media/downloads/A" {
		t.Fatalf("ApplyInverse longest prefix = %q, want /media/downloads/A", got)
	}
}

func TestRemapperApplyInversePrefersLongestLocalPrefix(t *testing.T) {
	r := Parse("/external/long:/books,/x:/books/downloads")

	if got := r.ApplyInverse("/books/downloads/A"); got != "/x/A" {
		t.Fatalf("ApplyInverse longest local prefix = %q, want /x/A", got)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("/media:/books, /abs:/audiobooks"); err != nil {
		t.Fatalf("Validate valid spec: %v", err)
	}
	if err := Validate("nodivider"); err == nil {
		t.Fatal("Validate invalid spec expected error")
	}
}

// TestParseWindowsDriveLetter covers the drive-designator colon: splitting an
// entry at the first colon turned `S:\Downloads:/downloads` into from="S",
// to="\Downloads:/downloads", so no Windows download client could ever be
// remapped (Discussion #1971).
func TestParseWindowsDriveLetter(t *testing.T) {
	r := Parse(`S:\Downloads:/downloads`)

	if len(r.rules) != 1 {
		t.Fatalf("Parse produced %d rules, want 1: %+v", len(r.rules), r.rules)
	}
	if r.rules[0].from != `S:\Downloads` || r.rules[0].to != "/downloads" {
		t.Fatalf("Parse = {from:%q to:%q}, want {from:%q to:%q}", r.rules[0].from, r.rules[0].to, `S:\Downloads`, "/downloads")
	}
}

func TestApplyWindowsSource(t *testing.T) {
	r := Parse(`S:\Downloads:/mnt/Storage/Downloads`)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"backslash path", `S:\Downloads\bindery\Book`, "/mnt/Storage/Downloads/bindery/Book"},
		{"forward-slash path as qBittorrent reports it", "S:/Downloads/Book", "/mnt/Storage/Downloads/Book"},
		{"drive letter and path are case-insensitive", `s:\downloads\Book`, "/mnt/Storage/Downloads/Book"},
		{"exact prefix match", `S:\Downloads`, "/mnt/Storage/Downloads"},
		{"trailing separator", `S:\Downloads\`, "/mnt/Storage/Downloads"},
		{"sibling prefix must not match", `S:\DownloadsExtra\Book`, `S:\DownloadsExtra\Book`},
		{"different drive is untouched", `D:\Downloads\Book`, `D:\Downloads\Book`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.Apply(tc.in); got != tc.want {
				t.Fatalf("Apply(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestApplyInverseWindowsRoundTrip proves the reverse direction reconstructs a
// path the Windows client can actually open, in the separator style the
// operator configured. torrentSavePath sends this back to qBittorrent.
func TestApplyInverseWindowsRoundTrip(t *testing.T) {
	r := Parse(`S:\Downloads:/mnt/Storage/Downloads`)

	got := r.ApplyInverse("/mnt/Storage/Downloads/bindery/Book")
	if want := `S:\Downloads\bindery\Book`; got != want {
		t.Fatalf("ApplyInverse = %q, want %q", got, want)
	}
	if back := r.Apply(got); back != "/mnt/Storage/Downloads/bindery/Book" {
		t.Fatalf("round trip = %q, want /mnt/Storage/Downloads/bindery/Book", back)
	}
	if got := r.ApplyInverse("/mnt/Storage/Downloads"); got != `S:\Downloads` {
		t.Fatalf("ApplyInverse exact = %q, want %q", got, `S:\Downloads`)
	}

	// A spec written with forward slashes keeps forward slashes.
	fwd := Parse("S:/Downloads:/mnt/Storage/Downloads")
	if got := fwd.ApplyInverse("/mnt/Storage/Downloads/Book"); got != "S:/Downloads/Book" {
		t.Fatalf("ApplyInverse forward-slash spec = %q, want S:/Downloads/Book", got)
	}
}

// TestApplyWindowsDestination covers the mirror topology: Bindery on Windows,
// the download client somewhere POSIX.
func TestApplyWindowsDestination(t *testing.T) {
	r := Parse(`/downloads:S:\Downloads`)

	if got := r.Apply("/downloads/bindery/Book"); got != `S:\Downloads\bindery\Book` {
		t.Fatalf("Apply = %q, want %q", got, `S:\Downloads\bindery\Book`)
	}
	if got := r.ApplyInverse(`S:\Downloads\bindery\Book`); got != "/downloads/bindery/Book" {
		t.Fatalf("ApplyInverse = %q, want /downloads/bindery/Book", got)
	}
	if got := r.ApplyInverse("s:/downloads/Book"); got != "/downloads/Book" {
		t.Fatalf("ApplyInverse case-insensitive = %q, want /downloads/Book", got)
	}
}

// TestPosixRulesStayCaseAndSeparatorSensitive pins the pre-existing behaviour:
// Linux is case-sensitive and a backslash is an ordinary filename byte, so the
// Windows leniency must not leak into POSIX-only specs.
func TestPosixRulesStayCaseAndSeparatorSensitive(t *testing.T) {
	r := Parse("/media/downloads:/books/downloads")

	if got := r.Apply("/Media/Downloads/A"); got != "/Media/Downloads/A" {
		t.Fatalf("Apply wrong case = %q, want it unchanged", got)
	}
	if got := r.Apply(`\media\downloads\A`); got != `\media\downloads\A` {
		t.Fatalf("Apply backslash path against POSIX rule = %q, want it unchanged", got)
	}
	// A backslash inside a POSIX remainder is data, not a separator.
	if got := r.Apply(`/media/downloads/od\d`); got != `/books/downloads/od\d` {
		t.Fatalf(`Apply POSIX remainder = %q, want /books/downloads/od\d`, got)
	}
}

func TestValidateWindows(t *testing.T) {
	if err := Validate(`S:\Downloads:/downloads`); err != nil {
		t.Fatalf("Validate Windows pair: %v", err)
	}
	if err := Validate(`/downloads:S:\Downloads`); err != nil {
		t.Fatalf("Validate Windows destination: %v", err)
	}
	// The reporter's mistake shape: only the client-side path, no destination.
	err := Validate(`S:\Downloads`)
	if err == nil {
		t.Fatal("Validate drive path with no destination expected error")
	}
	if !strings.Contains(err.Error(), "not in 'from:to' format") {
		t.Fatalf("Validate error %q does not keep the existing message style", err)
	}
	if !strings.Contains(err.Error(), `S:\Downloads:/downloads`) {
		t.Fatalf("Validate error %q does not show a working example", err)
	}
	if err := Validate(`S:\Downloads:`); err == nil {
		t.Fatal("Validate empty destination expected error")
	}
}

func TestIsWindowsPath(t *testing.T) {
	for _, in := range []string{`S:\Downloads`, "S:/Downloads", `s:\d`} {
		if !IsWindowsPath(in) {
			t.Fatalf("IsWindowsPath(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "/downloads", "S:", "SS:/x", "1:/x", "http://x/y"} {
		if IsWindowsPath(in) {
			t.Fatalf("IsWindowsPath(%q) = true, want false", in)
		}
	}
}
