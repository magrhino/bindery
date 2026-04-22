-- +migrate Up

-- One-shot deduplication of book rows created before title normalization
-- was applied symmetrically. Groups books by (author_id, lower(trim(title)))
-- and keeps the row with the best file state (ebook or audiobook file present),
-- breaking ties by lowest id. Re-parents series_books and history rows to the
-- surviving book, then deletes the duplicates.
--
-- NFC and umlaut normalization cannot be expressed in plain SQLite SQL, so this
-- migration handles only the whitespace and case variants. The application-level
-- NormalizeTitleForDedup function prevents new duplicates for all variant types.

-- Re-parent series_books rows that point to a loser book.
WITH winners AS (
    SELECT
        COALESCE(
            MIN(CASE WHEN ebook_file_path != '' OR audiobook_file_path != '' OR file_path != '' THEN id ELSE NULL END),
            MIN(id)
        ) AS winner_id,
        author_id,
        lower(trim(title)) AS norm
    FROM books
    GROUP BY author_id, lower(trim(title))
    HAVING COUNT(*) > 1
)
UPDATE series_books
SET book_id = (
    SELECT w.winner_id
    FROM books b
    JOIN winners w ON w.author_id = b.author_id AND w.norm = lower(trim(b.title))
    WHERE b.id = series_books.book_id
)
WHERE book_id IN (
    SELECT b.id
    FROM books b
    JOIN winners w ON w.author_id = b.author_id AND w.norm = lower(trim(b.title))
    WHERE b.id != w.winner_id
);

-- Re-parent history rows that point to a loser book.
WITH winners AS (
    SELECT
        COALESCE(
            MIN(CASE WHEN ebook_file_path != '' OR audiobook_file_path != '' OR file_path != '' THEN id ELSE NULL END),
            MIN(id)
        ) AS winner_id,
        author_id,
        lower(trim(title)) AS norm
    FROM books
    GROUP BY author_id, lower(trim(title))
    HAVING COUNT(*) > 1
)
UPDATE history
SET book_id = (
    SELECT w.winner_id
    FROM books b
    JOIN winners w ON w.author_id = b.author_id AND w.norm = lower(trim(b.title))
    WHERE b.id = history.book_id
)
WHERE book_id IN (
    SELECT b.id
    FROM books b
    JOIN winners w ON w.author_id = b.author_id AND w.norm = lower(trim(b.title))
    WHERE b.id != w.winner_id
);

-- Delete the loser rows.
WITH winners AS (
    SELECT
        COALESCE(
            MIN(CASE WHEN ebook_file_path != '' OR audiobook_file_path != '' OR file_path != '' THEN id ELSE NULL END),
            MIN(id)
        ) AS winner_id,
        author_id,
        lower(trim(title)) AS norm
    FROM books
    GROUP BY author_id, lower(trim(title))
    HAVING COUNT(*) > 1
)
DELETE FROM books
WHERE id IN (
    SELECT b.id
    FROM books b
    JOIN winners w ON w.author_id = b.author_id AND w.norm = lower(trim(b.title))
    WHERE b.id != w.winner_id
);

-- +migrate Down
-- Data-loss migration; down path is intentionally a no-op.
