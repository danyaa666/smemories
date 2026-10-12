-- +goose Up
-- T-066 (docs/db-conventions.md): note_photos -> note_photo_tab; no foreign keys (the notes service deletes the rows with the note,
-- ClearMediaRefs with the photo), surrogate id BIGINT UNSIGNED AUTO_INCREMENT as primary key (the old key (note_id, position)
-- becomes the unique key uq_note_photo_tab_note_id_sort_order), position -> sort_order (position is a MySQL function name),
-- created_at / updated_at BIGINT ms copied from the note. note_id is indexed by that unique key, media_id by its own.
RENAME TABLE note_photos TO note_photo_tab;
ALTER TABLE note_photo_tab DROP FOREIGN KEY fk_note_photos_note, DROP FOREIGN KEY fk_note_photos_media;
ALTER TABLE note_photo_tab RENAME COLUMN position TO sort_order;
ALTER TABLE note_photo_tab
  DROP PRIMARY KEY,
  ADD COLUMN id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT FIRST,
  ADD PRIMARY KEY (id),
  ADD UNIQUE KEY uq_note_photo_tab_note_id_sort_order (note_id, sort_order),
  RENAME INDEX uq_note_photos_media TO uq_note_photo_tab_media_id,
  ADD COLUMN created_at BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;
UPDATE note_photo_tab np JOIN note_tab n ON n.id = np.note_id SET np.created_at = n.created_at, np.updated_at = n.created_at;
ALTER TABLE note_photo_tab ALTER COLUMN created_at DROP DEFAULT, ALTER COLUMN updated_at DROP DEFAULT;

-- +goose Down
-- Restores the schema and the rows. Photo links of a missing note or photo, which only the foreign keys would have prevented,
-- are deleted first.
DELETE FROM note_photo_tab WHERE note_id NOT IN (SELECT id FROM note_tab) OR media_id NOT IN (SELECT id FROM media_tab);
ALTER TABLE note_photo_tab
  DROP INDEX uq_note_photo_tab_note_id_sort_order,
  DROP PRIMARY KEY,
  DROP COLUMN id,
  DROP COLUMN created_at,
  DROP COLUMN updated_at;
ALTER TABLE note_photo_tab RENAME COLUMN sort_order TO position;
ALTER TABLE note_photo_tab
  ADD PRIMARY KEY (note_id, position),
  RENAME INDEX uq_note_photo_tab_media_id TO uq_note_photos_media,
  ADD CONSTRAINT fk_note_photos_note FOREIGN KEY (note_id) REFERENCES note_tab (id) ON DELETE CASCADE,
  ADD CONSTRAINT fk_note_photos_media FOREIGN KEY (media_id) REFERENCES media_tab (id) ON DELETE CASCADE;
RENAME TABLE note_photo_tab TO note_photos;
