-- +goose Up
-- T-066 (docs/db-conventions.md): notes -> note_tab; no foreign keys (the notes service deletes the rows in the yearbook delete
-- transaction), status VARCHAR(16) instead of ENUM (pending, approved, hidden; validated in Go, set by the insert), created_at BIGINT
-- Unix ms, updated_at new (= created_at for existing rows). note_photos keeps its foreign key to this table until 20261012100200.
-- Sub-millisecond digits of the old DATETIME(6) values (UTC) are dropped.
RENAME TABLE notes TO note_tab;
ALTER TABLE note_tab DROP FOREIGN KEY fk_notes_collection, DROP FOREIGN KEY fk_notes_yearbook;
ALTER TABLE note_tab
  RENAME INDEX uq_notes_public_id TO uq_note_tab_public_id,
  RENAME INDEX ix_notes_collection TO idx_collection_id,
  RENAME INDEX ix_notes_yearbook_status_order TO idx_yearbook_id_status_sort_order,
  MODIFY status VARCHAR(16) NOT NULL,
  ADD COLUMN created_at_ms BIGINT NULL;
UPDATE note_tab SET created_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', created_at) DIV 1000;
ALTER TABLE note_tab DROP COLUMN created_at;
ALTER TABLE note_tab RENAME COLUMN created_at_ms TO created_at;
ALTER TABLE note_tab
  MODIFY created_at BIGINT NOT NULL,
  ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;
UPDATE note_tab SET updated_at = created_at;
ALTER TABLE note_tab ALTER COLUMN updated_at DROP DEFAULT;

-- +goose Down
-- Restores the schema and the values. Notes of a missing collection or yearbook, which only the foreign keys would have
-- prevented, are deleted first (their photo links follow through the note_photos foreign key).
DELETE FROM note_tab WHERE collection_id NOT IN (SELECT id FROM note_collection_tab) OR yearbook_id NOT IN (SELECT id FROM yearbook_tab);
ALTER TABLE note_tab
  DROP COLUMN updated_at,
  ADD COLUMN created_at_dt DATETIME(6) NULL;
UPDATE note_tab SET created_at_dt = TIMESTAMPADD(MICROSECOND, created_at * 1000, '1970-01-01 00:00:00');
ALTER TABLE note_tab DROP COLUMN created_at;
ALTER TABLE note_tab RENAME COLUMN created_at_dt TO created_at;
ALTER TABLE note_tab
  MODIFY created_at DATETIME(6) NOT NULL,
  MODIFY status ENUM('pending','approved','hidden') NOT NULL DEFAULT 'pending',
  RENAME INDEX uq_note_tab_public_id TO uq_notes_public_id,
  RENAME INDEX idx_collection_id TO ix_notes_collection,
  RENAME INDEX idx_yearbook_id_status_sort_order TO ix_notes_yearbook_status_order,
  ADD CONSTRAINT fk_notes_collection FOREIGN KEY (collection_id) REFERENCES note_collection_tab (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_notes_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbook_tab (id) ON DELETE CASCADE;
RENAME TABLE note_tab TO notes;
