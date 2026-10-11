-- +goose Up
-- T-064 (docs/db-conventions.md): yearbooks -> yearbook_tab, profiles -> profile_tab; no foreign keys (the yearbook service
-- deletes the children and clears media references), BIGINT Unix-ms timestamps, VARCHAR instead of ENUM.
-- The old DATETIME(6) values are UTC; TIMESTAMPDIFF does not depend on the session time zone. Sub-millisecond digits are
-- dropped (a negative value truncates towards zero). There is no production data, so one ALTER per step is fine.
-- Other tables (media, note_collections, notes) keep their foreign keys to yearbook_tab until their own conversion tasks:
-- RENAME TABLE repoints them.
RENAME TABLE yearbooks TO yearbook_tab, profiles TO profile_tab;

ALTER TABLE profile_tab DROP FOREIGN KEY fk_profiles_yearbook, DROP FOREIGN KEY fk_profiles_photo_media;
ALTER TABLE yearbook_tab DROP FOREIGN KEY fk_yearbooks_owner, DROP FOREIGN KEY fk_yearbooks_cover_media;

-- yearbook_tab: the (owner_id, updated_at) index and the index the cover foreign key created go; both come back below
-- (the first one with public_id as its last column).
ALTER TABLE yearbook_tab
  DROP INDEX ix_yearbooks_owner_updated,
  DROP INDEX fk_yearbooks_cover_media,
  RENAME INDEX uq_yearbooks_public_id TO uq_yearbook_tab_public_id,
  MODIFY language  VARCHAR(8) NOT NULL,
  MODIFY page_size VARCHAR(8) NOT NULL DEFAULT 'A5',
  ADD COLUMN created_at_ms BIGINT NULL,
  ADD COLUMN updated_at_ms BIGINT NULL;
UPDATE yearbook_tab SET
  created_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', created_at) DIV 1000,
  updated_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', updated_at) DIV 1000;
ALTER TABLE yearbook_tab DROP COLUMN created_at, DROP COLUMN updated_at;
ALTER TABLE yearbook_tab RENAME COLUMN created_at_ms TO created_at, RENAME COLUMN updated_at_ms TO updated_at;
ALTER TABLE yearbook_tab
  MODIFY created_at BIGINT NOT NULL,
  MODIFY updated_at BIGINT NOT NULL,
  ADD INDEX idx_owner_updated (owner_id, updated_at, public_id), -- serves the list order (updated_at, public_id) descending without a sort
  ADD INDEX idx_cover_media_id (cover_media_id);

-- profile_tab: created_at/updated_at are new and copied from the book. owner_flag (1 for the owner's profile, NULL otherwise,
-- so the unique key allows one owner profile per book) is rebuilt around the new is_owner type.
ALTER TABLE profile_tab
  DROP INDEX uq_profiles_one_owner,
  DROP INDEX fk_profiles_photo_media,
  DROP COLUMN owner_flag,
  RENAME INDEX uq_profiles_public_id TO uq_profile_tab_public_id,
  MODIFY is_owner TINYINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN created_at BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;
UPDATE profile_tab p JOIN yearbook_tab y ON y.id = p.yearbook_id SET p.created_at = y.created_at, p.updated_at = y.updated_at;
ALTER TABLE profile_tab
  ALTER COLUMN created_at DROP DEFAULT,
  ALTER COLUMN updated_at DROP DEFAULT,
  ADD COLUMN owner_flag TINYINT GENERATED ALWAYS AS (IF(is_owner, 1, NULL)) STORED,
  ADD UNIQUE KEY uq_profile_tab_one_owner (yearbook_id, owner_flag),
  ADD INDEX idx_photo_media_id (photo_media_id);

-- +goose Down
-- Restores the schema and the values (milliseconds back to DATETIME(6)). Rows that only the foreign keys would have
-- prevented are removed first: dangling media references become NULL, profiles of a missing book and books of a missing
-- user are deleted.
ALTER TABLE profile_tab
  DROP INDEX uq_profile_tab_one_owner,
  DROP INDEX idx_photo_media_id,
  DROP COLUMN owner_flag,
  DROP COLUMN created_at,
  DROP COLUMN updated_at,
  RENAME INDEX uq_profile_tab_public_id TO uq_profiles_public_id,
  MODIFY is_owner BOOLEAN NOT NULL;
ALTER TABLE profile_tab
  ADD COLUMN owner_flag TINYINT GENERATED ALWAYS AS (IF(is_owner, 1, NULL)) STORED,
  ADD UNIQUE KEY uq_profiles_one_owner (yearbook_id, owner_flag);

ALTER TABLE yearbook_tab
  ADD COLUMN created_at_dt DATETIME(6) NULL,
  ADD COLUMN updated_at_dt DATETIME(6) NULL;
UPDATE yearbook_tab SET
  created_at_dt = TIMESTAMPADD(MICROSECOND, created_at * 1000, '1970-01-01 00:00:00'),
  updated_at_dt = TIMESTAMPADD(MICROSECOND, updated_at * 1000, '1970-01-01 00:00:00');
ALTER TABLE yearbook_tab
  DROP INDEX idx_owner_updated,
  DROP INDEX idx_cover_media_id,
  DROP COLUMN created_at,
  DROP COLUMN updated_at,
  RENAME INDEX uq_yearbook_tab_public_id TO uq_yearbooks_public_id,
  MODIFY language  ENUM('en','vi') NOT NULL,
  MODIFY page_size ENUM('A5','A4','Letter') NOT NULL DEFAULT 'A5';
ALTER TABLE yearbook_tab RENAME COLUMN created_at_dt TO created_at, RENAME COLUMN updated_at_dt TO updated_at;
ALTER TABLE yearbook_tab
  MODIFY created_at DATETIME(6) NOT NULL,
  MODIFY updated_at DATETIME(6) NOT NULL,
  ADD INDEX ix_yearbooks_owner_updated (owner_id, updated_at);

UPDATE yearbook_tab SET cover_media_id = NULL WHERE cover_media_id IS NOT NULL AND cover_media_id NOT IN (SELECT id FROM media);
UPDATE profile_tab SET photo_media_id = NULL WHERE photo_media_id IS NOT NULL AND photo_media_id NOT IN (SELECT id FROM media);
DELETE FROM profile_tab WHERE yearbook_id NOT IN (SELECT id FROM yearbook_tab);
DELETE FROM yearbook_tab WHERE owner_id NOT IN (SELECT id FROM users);
DELETE FROM profile_tab WHERE yearbook_id NOT IN (SELECT id FROM yearbook_tab);
ALTER TABLE yearbook_tab
  ADD CONSTRAINT fk_yearbooks_owner FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_yearbooks_cover_media FOREIGN KEY (cover_media_id) REFERENCES media (id) ON DELETE SET NULL;
ALTER TABLE profile_tab
  ADD CONSTRAINT fk_profiles_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbook_tab (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_profiles_photo_media FOREIGN KEY (photo_media_id) REFERENCES media (id) ON DELETE SET NULL;

RENAME TABLE yearbook_tab TO yearbooks, profile_tab TO profiles;
