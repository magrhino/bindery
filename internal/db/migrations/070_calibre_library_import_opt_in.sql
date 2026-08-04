-- +migrate Up
-- Calibre library import is now opt-in via calibre.library_import_enabled.
--
-- The setting existed in the UI but no backend code read it, so startup, the
-- 24h scheduler sync, and the manual import ran off calibre.library_path +
-- sync_on_startup regardless — a user who saw the toggle "off" was still being
-- imported (#calibre-import-opt-in). The backend now gates every import path on
-- the setting.
--
-- To avoid disabling an import that was already working, backfill the flag to
-- 'true' for installs that already have a library path configured. Fresh
-- installs leave it unset (default off), so a newly-configured Calibre library
-- imports only after the operator explicitly enables the toggle.
INSERT INTO settings (key, value)
SELECT 'calibre.library_import_enabled', 'true'
WHERE EXISTS (
    SELECT 1 FROM settings
    WHERE key = 'calibre.library_path' AND TRIM(value) <> ''
)
AND NOT EXISTS (
    SELECT 1 FROM settings WHERE key = 'calibre.library_import_enabled'
);

-- +migrate Down
DELETE FROM settings WHERE key = 'calibre.library_import_enabled';
