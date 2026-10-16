-- +goose Up
-- T-081 (D-28): a unique public username per user. username_changed_at is the Unix ms of the last user-made change, 0 = never changed.
-- Backfill before the unique key: user- plus the last 8 characters of the ULID, lower-cased. The tail of a ULID is random, so a clash is
-- very unlikely; if there is one, every row after the first of a clashing group gets the row id appended (cut so the name stays within
-- 30 characters), so the migration never fails on it. The column is ascii: the allowed characters are a-z 0-9 . _ - and the (ci)
-- collation makes the unique key case-insensitive, though the API only stores lower case.
ALTER TABLE user_tab
  ADD COLUMN username VARCHAR(30) CHARACTER SET ascii NULL,
  ADD COLUMN username_changed_at BIGINT NOT NULL DEFAULT 0;
UPDATE user_tab SET username = CONCAT('user-', LOWER(RIGHT(public_id, 8)));
UPDATE user_tab u
  JOIN (SELECT username, MIN(id) AS keep_id FROM user_tab GROUP BY username HAVING COUNT(*) > 1) d ON d.username = u.username AND u.id <> d.keep_id
  SET u.username = CONCAT(LEFT(u.username, 29 - LENGTH(u.id)), '-', u.id);
ALTER TABLE user_tab
  MODIFY username VARCHAR(30) CHARACTER SET ascii NOT NULL,
  ADD UNIQUE KEY uq_user_tab_username (username);

-- +goose Down
ALTER TABLE user_tab
  DROP INDEX uq_user_tab_username,
  DROP COLUMN username_changed_at,
  DROP COLUMN username;
