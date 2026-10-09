-- +goose Up
-- Friends' notes (T-034). A note belongs to the collection link it came through and to its yearbook; deleting either
-- deletes it. answers holds the cleaned text by note-field id (internal/notefields), so a template can change its
-- fields without a schema change. No IP address and no user agent is stored.
CREATE TABLE notes (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  public_id     CHAR(26)        NOT NULL,
  collection_id BIGINT UNSIGNED NOT NULL,
  yearbook_id   BIGINT UNSIGNED NOT NULL,
  answers       JSON            NOT NULL,
  status        ENUM('pending','approved','hidden') NOT NULL DEFAULT 'pending',
  sort_order    INT             NULL COMMENT 'null until approved and ordered',
  created_at    DATETIME(6)     NOT NULL,
  UNIQUE KEY uq_notes_public_id (public_id),
  KEY ix_notes_collection (collection_id),
  KEY ix_notes_yearbook_status_order (yearbook_id, status, sort_order),
  CONSTRAINT fk_notes_collection FOREIGN KEY (collection_id) REFERENCES note_collections (id) ON DELETE CASCADE,
  CONSTRAINT fk_notes_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbooks (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Up to three photos per note (enforced by the API); position is 0-2. A photo belongs to one note.
CREATE TABLE note_photos (
  note_id   BIGINT UNSIGNED NOT NULL,
  media_id  BIGINT UNSIGNED NOT NULL,
  position  TINYINT UNSIGNED NOT NULL,
  PRIMARY KEY (note_id, position),
  UNIQUE KEY uq_note_photos_media (media_id),
  CONSTRAINT fk_note_photos_note FOREIGN KEY (note_id) REFERENCES notes (id) ON DELETE CASCADE,
  CONSTRAINT fk_note_photos_media FOREIGN KEY (media_id) REFERENCES media (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose Down
DROP TABLE note_photos;
DROP TABLE notes;
