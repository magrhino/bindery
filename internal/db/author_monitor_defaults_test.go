package db

import (
	"context"
	"testing"

	"github.com/vavallee/bindery/internal/models"
)

func TestResolveAuthorMonitorDefaults(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	settings := NewSettingsRepo(database)
	ctx := context.Background()

	cases := []struct {
		name            string
		mode            string
		latest          string
		setMode         bool
		setLatest       bool
		wantMode        string
		wantLatestCount int
	}{
		{name: "unset falls back to built-in defaults", wantMode: models.AuthorMonitorModeAll, wantLatestCount: 1},
		{name: "none", setMode: true, mode: models.AuthorMonitorModeNone, wantMode: models.AuthorMonitorModeNone, wantLatestCount: 1},
		{name: "future", setMode: true, mode: models.AuthorMonitorModeFuture, wantMode: models.AuthorMonitorModeFuture, wantLatestCount: 1},
		{name: "latest with count", setMode: true, mode: models.AuthorMonitorModeLatest, setLatest: true, latest: "3", wantMode: models.AuthorMonitorModeLatest, wantLatestCount: 3},
		{name: "empty value is not a mode", setMode: true, mode: "", wantMode: models.AuthorMonitorModeAll, wantLatestCount: 1},
		{name: "garbage value is ignored", setMode: true, mode: "yesterday", wantMode: models.AuthorMonitorModeAll, wantLatestCount: 1},
		// "series" pins an author to a hand-picked set of series (#810) — there
		// is no install-wide equivalent, so the read side refuses it the same
		// way the settings validator refuses to store it.
		{name: "series is rejected as a global default", setMode: true, mode: models.AuthorMonitorModeSeries, wantMode: models.AuthorMonitorModeAll, wantLatestCount: 1},
		{name: "non-numeric latest count ignored", setLatest: true, latest: "many", wantMode: models.AuthorMonitorModeAll, wantLatestCount: 1},
		{name: "zero latest count ignored", setLatest: true, latest: "0", wantMode: models.AuthorMonitorModeAll, wantLatestCount: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset both keys so cases don't leak into each other.
			if err := settings.Set(ctx, SettingAuthorDefaultMonitorMode, ""); err != nil {
				t.Fatal(err)
			}
			if err := settings.Set(ctx, SettingAuthorDefaultMonitorLatestCount, ""); err != nil {
				t.Fatal(err)
			}
			if tc.setMode {
				if err := settings.Set(ctx, SettingAuthorDefaultMonitorMode, tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			if tc.setLatest {
				if err := settings.Set(ctx, SettingAuthorDefaultMonitorLatestCount, tc.latest); err != nil {
					t.Fatal(err)
				}
			}

			mode, latestCount := ResolveAuthorMonitorDefaults(ctx, settings)
			if mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", mode, tc.wantMode)
			}
			if latestCount != tc.wantLatestCount {
				t.Errorf("latestCount = %d, want %d", latestCount, tc.wantLatestCount)
			}
		})
	}
}

func TestResolveAuthorMonitorDefaultsNilRepo(t *testing.T) {
	mode, latestCount := ResolveAuthorMonitorDefaults(context.Background(), nil)
	if mode != models.AuthorMonitorModeAll || latestCount != 1 {
		t.Fatalf("nil settings = (%q, %d), want (all, 1)", mode, latestCount)
	}
}

// A caller that already decided on a mode keeps it — the Hardcover list syncer
// pins "none" (#1290) and the add-author handler passes the request's choice.
// Only the zero value gets filled in from settings.
func TestApplyAuthorMonitorDefaultsRespectsExplicitChoice(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	settings := NewSettingsRepo(database)
	ctx := context.Background()

	if err := settings.Set(ctx, SettingAuthorDefaultMonitorMode, models.AuthorMonitorModeNone); err != nil {
		t.Fatal(err)
	}

	explicit := &models.Author{Name: "Pinned", MonitorMode: models.AuthorMonitorModeFuture, MonitorLatestCount: 7}
	ApplyAuthorMonitorDefaults(ctx, settings, explicit)
	if explicit.MonitorMode != models.AuthorMonitorModeFuture {
		t.Errorf("explicit MonitorMode = %q, want it left alone", explicit.MonitorMode)
	}
	if explicit.MonitorLatestCount != 7 {
		t.Errorf("explicit MonitorLatestCount = %d, want 7", explicit.MonitorLatestCount)
	}

	// Series is a legitimate per-author choice even though it's refused as an
	// install-wide default, so it must survive too.
	series := &models.Author{Name: "Series", MonitorMode: models.AuthorMonitorModeSeries}
	ApplyAuthorMonitorDefaults(ctx, settings, series)
	if series.MonitorMode != models.AuthorMonitorModeSeries {
		t.Errorf("series MonitorMode = %q, want it left alone", series.MonitorMode)
	}

	unset := &models.Author{Name: "Unset"}
	ApplyAuthorMonitorDefaults(ctx, settings, unset)
	if unset.MonitorMode != models.AuthorMonitorModeNone {
		t.Errorf("unset MonitorMode = %q, want %q from settings", unset.MonitorMode, models.AuthorMonitorModeNone)
	}
	if unset.MonitorLatestCount != 1 {
		t.Errorf("unset MonitorLatestCount = %d, want 1", unset.MonitorLatestCount)
	}

	ApplyAuthorMonitorDefaults(ctx, settings, nil) // must not panic
}
