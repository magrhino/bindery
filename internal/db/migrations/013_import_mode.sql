-- +migrate Up

-- Import mode controls how Bindery places completed downloads into the library.
-- "move"     — rename/move the file (existing behaviour, source is removed)
-- "copy"     — copy file to library, source remains (seeding continues, double disk use)
-- "hardlink" — hard-link file into library, source inode is shared (zero extra disk, seeding continues)
INSERT OR IGNORE INTO settings (key, value) VALUES ('import.mode', 'move');

-- +migrate Down
DELETE FROM settings WHERE key = 'import.mode';
