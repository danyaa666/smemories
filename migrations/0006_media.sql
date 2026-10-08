-- +goose Up
-- Uploaded photos. Rows are owned by a yearbook (deleting the book deletes its rows; the objects are removed
-- from storage first by the API). Storage keys are generated, never derived from file names.
CREATE TABLE media (
  id            BIGINT UNSIGNED       NOT NULL AUTO_INCREMENT PRIMARY KEY,
  public_id     CHAR(26)              NOT NULL,
  yearbook_id   BIGINT UNSIGNED       NOT NULL,
  uploader_kind ENUM('owner','contributor') NOT NULL,
  object_key    VARCHAR(255)          NOT NULL,
  thumb_key     VARCHAR(255)          NOT NULL,
  content_type  VARCHAR(32)           NOT NULL COMMENT 'of the display object',
  bytes         INT UNSIGNED          NOT NULL COMMENT 'size of the display object',
  width         SMALLINT UNSIGNED     NOT NULL,
  height        SMALLINT UNSIGNED     NOT NULL,
  sha256        CHAR(64)              NOT NULL COMMENT 'hex, of the display object',
  created_at    DATETIME(6)           NOT NULL,
  UNIQUE KEY uq_media_public_id (public_id),
  KEY ix_media_yearbook (yearbook_id),
  CONSTRAINT fk_media_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbooks (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE yearbooks
  ADD COLUMN cover_media_id BIGINT UNSIGNED NULL AFTER template_id,
  ADD CONSTRAINT fk_yearbooks_cover_media FOREIGN KEY (cover_media_id) REFERENCES media (id) ON DELETE SET NULL;

ALTER TABLE profiles
  ADD CONSTRAINT fk_profiles_photo_media FOREIGN KEY (photo_media_id) REFERENCES media (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE profiles DROP FOREIGN KEY fk_profiles_photo_media;
ALTER TABLE yearbooks DROP FOREIGN KEY fk_yearbooks_cover_media, DROP COLUMN cover_media_id;
DROP TABLE media;
