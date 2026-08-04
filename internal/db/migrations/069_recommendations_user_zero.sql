-- +migrate Up
-- Re-parent recommendation rows stranded under user 0 (#1725).
--
-- The local-only and API-key auth branches set the caller's role but never the
-- caller's user id, so UserIDFromContext returned 0 and a refresh triggered
-- from a trusted-local browser or a script wrote its batch under user 0, while
-- the nightly scheduler job wrote under a hardcoded id. Recommendations and
-- dismissals are both user-scoped, so the two never saw each other: dismissed
-- books reappeared, and a batch generated one way was invisible the other.
--
-- Rows under user 0 are unreachable — no session can read or dismiss them —
-- so hand them to the operator (the lowest-id admin, the same identity the
-- auth layer now resolves). Guarded on an admin existing; on an install with
-- none, the rows stay put and the next refresh replaces them anyway.
UPDATE recommendations
SET user_id = (SELECT MIN(id) FROM users WHERE role = 'admin')
WHERE user_id = 0
  AND EXISTS (SELECT 1 FROM users WHERE role = 'admin');

-- Dismissals carry (user_id, foreign_id) with a uniqueness constraint, so a
-- straight UPDATE can collide with a dismissal the admin already recorded for
-- the same book. Move only the rows that do not collide, then drop the rest:
-- a duplicate dismissal carries no information the surviving row lacks.
UPDATE OR IGNORE recommendation_dismissals
SET user_id = (SELECT MIN(id) FROM users WHERE role = 'admin')
WHERE user_id = 0
  AND EXISTS (SELECT 1 FROM users WHERE role = 'admin');

DELETE FROM recommendation_dismissals
WHERE user_id = 0
  AND EXISTS (SELECT 1 FROM users WHERE role = 'admin');

-- +migrate Down
-- Irreversible: user 0 is not a real account, and which rows originated there
-- is not recorded once they are re-parented.
