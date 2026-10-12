-- +goose Up
-- T-065 (docs/db-conventions.md): media -> media_tab; no foreign key to yearbook_tab (the yearbook service deletes the rows,
-- media.Store.removeRow clears the cover and profile references), uploader_kind VARCHAR(16) instead of ENUM (validated in
-- Go), created_at BIGINT Unix ms (updated_at = created_at for existing rows), idx_yearbook_id instead of ix_media_yearbook.
-- Column names checked against information_schema.KEYWORDS (RESERVED = 1): bytes, width, height, sha256 are not reserved.
-- note_photos still has its foreign key to media; RENAME TABLE repoints it until the notes conversion task removes it.
-- The old DATETIME(6) values are UTC; TIMESTAMPDIFF does not depend on the session time zone. Sub-millisecond digits are dropped.
RENAME TABLE media TO media_tab;
ALTER TABLE media_tab DROP FOREIGN KEY fk_media_yearbook;
ALTER TABLE media_tab
  RENAME INDEX ix_media_yearbook TO idx_yearbook_id,
  RENAME INDEX uq_media_public_id TO uq_media_tab_public_id,
  MODIFY uploader_kind VARCHAR(16) NOT NULL,
  ADD COLUMN created_at_ms BIGINT NULL;
UPDATE media_tab SET created_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', created_at) DIV 1000;
ALTER TABLE media_tab DROP COLUMN created_at;
ALTER TABLE media_tab RENAME COLUMN created_at_ms TO created_at;
ALTER TABLE media_tab
  MODIFY created_at BIGINT NOT NULL,
  ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;
UPDATE media_tab SET updated_at = created_at;
ALTER TABLE media_tab ALTER COLUMN updated_at DROP DEFAULT;

-- +goose Down
-- Restores the schema and the values (milliseconds back to DATETIME(6)). Rows of a missing yearbook, which only the foreign
-- key would have prevented, are deleted first (note_photos rows follow through their own foreign key).
DELETE FROM media_tab WHERE yearbook_id NOT IN (SELECT id FROM yearbook_tab);
ALTER TABLE media_tab
  DROP COLUMN updated_at,
  ADD COLUMN created_at_dt DATETIME(6) NULL;
UPDATE media_tab SET created_at_dt = TIMESTAMPADD(MICROSECOND, created_at * 1000, '1970-01-01 00:00:00');
ALTER TABLE media_tab DROP COLUMN created_at;
ALTER TABLE media_tab RENAME COLUMN created_at_dt TO created_at;
ALTER TABLE media_tab
  MODIFY created_at DATETIME(6) NOT NULL,
  MODIFY uploader_kind ENUM('owner','contributor') NOT NULL,
  RENAME INDEX idx_yearbook_id TO ix_media_yearbook,
  RENAME INDEX uq_media_tab_public_id TO uq_media_public_id,
  ADD CONSTRAINT fk_media_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbook_tab (id) ON DELETE CASCADE;
RENAME TABLE media_tab TO media;
