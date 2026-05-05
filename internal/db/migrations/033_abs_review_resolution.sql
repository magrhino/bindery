-- +migrate Up

-- Folded into 032 before release. Keep this migration as a no-op so branch
-- databases that already recorded version 033 remain coherent, and re-running
-- version 033 cannot duplicate columns.

-- +migrate Down

-- No-op; the 033 changes were folded into 032 before release.
