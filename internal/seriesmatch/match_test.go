package seriesmatch

import "testing"

func TestTitleScoreUnicodeDoesNotPanic(t *testing.T) {
	score := TitleScore("Cien años de soledad", "Cien años de soledad / One Hundred Years of Solitude")
	if score <= 0 {
		t.Fatalf("TitleScore returned %d, want positive score", score)
	}
}

func TestCleanTitleNormalizesUnicode(t *testing.T) {
	got := CleanTitle("Go\u0308del")
	want := CleanTitle("Gödel")
	if got != want {
		t.Fatalf("CleanTitle decomposed = %q, precomposed = %q", got, want)
	}
}

func TestTitleScoreASCIIWithSubtitleNoise(t *testing.T) {
	score := TitleScore("Project Hail Mary: A Novel", "Project Hail Mary")
	if score < 90 {
		t.Fatalf("TitleScore = %d, want high subtitle/noise match", score)
	}
}
