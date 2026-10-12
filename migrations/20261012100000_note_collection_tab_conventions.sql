-- +goose Up
-- T-066 (docs/db-conventions.md): note_collections -> note_collection_tab; no foreign key to yearbook_tab (the notes service
-- deletes the rows in the yearbook delete transaction), deadline_at / revoked_at / created_at are BIGINT Unix ms, updated_at is new
-- (the later of created_at and revoked_at for existing rows). The notes table keeps its foreign key to this table until its own
-- migration (20261012100100); RENAME TABLE repoints it.
-- The old DATETIME(6) values are UTC; TIMESTAMPDIFF does not depend on the session time zone. Sub-millisecond digits are dropped
-- (a negative value truncates towards zero); NULL deadlines and revocations stay NULL.
RENAME TABLE note_collections TO note_collection_tab;
ALTER TABLE note_collection_tab DROP FOREIGN KEY fk_note_collections_yearbook;
ALTER TABLE note_collection_tab
  DROP INDEX ix_note_collections_yearbook,
  RENAME INDEX uq_note_collections_public_id TO uq_note_collection_tab_public_id,
  RENAME INDEX uq_note_collections_token_hash TO uq_note_collection_tab_token_hash,
  ADD COLUMN created_at_ms BIGINT NULL,
  ADD COLUMN deadline_at_ms BIGINT NULL,
  ADD COLUMN revoked_at_ms BIGINT NULL;
UPDATE note_collection_tab SET
  created_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', created_at) DIV 1000,
  deadline_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', deadline_at) DIV 1000,
  revoked_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', revoked_at) DIV 1000;
ALTER TABLE note_collection_tab DROP COLUMN created_at, DROP COLUMN deadline_at, DROP COLUMN revoked_at;
ALTER TABLE note_collection_tab
  RENAME COLUMN created_at_ms TO created_at,
  RENAME COLUMN deadline_at_ms TO deadline_at,
  RENAME COLUMN revoked_at_ms TO revoked_at;
ALTER TABLE note_collection_tab
  MODIFY created_at BIGINT NOT NULL,
  ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0,
  ADD INDEX idx_yearbook_id_created_at (yearbook_id, created_at);
UPDATE note_collection_tab SET updated_at = IF(revoked_at IS NULL, created_at, GREATEST(created_at, revoked_at));
ALTER TABLE note_collection_tab ALTER COLUMN updated_at DROP DEFAULT;

-- +goose Down
-- Restores the schema and the values (milliseconds back to DATETIME(6)). Collections of a missing yearbook, which only the
-- foreign key would have prevented, are deleted first (their notes follow through the notes foreign key).
DELETE FROM note_collection_tab WHERE yearbook_id NOT IN (SELECT id FROM yearbook_tab);
ALTER TABLE note_collection_tab
  DROP INDEX idx_yearbook_id_created_at,
  DROP COLUMN updated_at,
  ADD COLUMN created_at_dt DATETIME(6) NULL,
  ADD COLUMN deadline_at_dt DATETIME(6) NULL,
  ADD COLUMN revoked_at_dt DATETIME(6) NULL;
UPDATE note_collection_tab SET
  created_at_dt = TIMESTAMPADD(MICROSECOND, created_at * 1000, '1970-01-01 00:00:00'),
  deadline_at_dt = TIMESTAMPADD(MICROSECOND, deadline_at * 1000, '1970-01-01 00:00:00'),
  revoked_at_dt = TIMESTAMPADD(MICROSECOND, revoked_at * 1000, '1970-01-01 00:00:00');
ALTER TABLE note_collection_tab DROP COLUMN created_at, DROP COLUMN deadline_at, DROP COLUMN revoked_at;
ALTER TABLE note_collection_tab
  RENAME COLUMN created_at_dt TO created_at,
  RENAME COLUMN deadline_at_dt TO deadline_at,
  RENAME COLUMN revoked_at_dt TO revoked_at;
ALTER TABLE note_collection_tab
  MODIFY created_at DATETIME(6) NOT NULL,
  RENAME INDEX uq_note_collection_tab_public_id TO uq_note_collections_public_id,
  RENAME INDEX uq_note_collection_tab_token_hash TO uq_note_collections_token_hash,
  ADD INDEX ix_note_collections_yearbook (yearbook_id, created_at),
  ADD CONSTRAINT fk_note_collections_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbook_tab (id) ON DELETE CASCADE;
RENAME TABLE note_collection_tab TO note_collections;
