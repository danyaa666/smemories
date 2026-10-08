-- +goose Up
-- Collection links (T-012): a private, revocable link per yearbook. Only the SHA-256 of the link token is stored.
-- Deleting a yearbook deletes its links. deadline_at NULL = no deadline; revoked_at NULL = active.
CREATE TABLE note_collections (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  public_id   CHAR(26)        NOT NULL,
  yearbook_id BIGINT UNSIGNED NOT NULL,
  token_hash  BINARY(32)      NOT NULL COMMENT 'sha256 of the token, which is never stored',
  label       VARCHAR(60)     NOT NULL DEFAULT '',
  deadline_at DATETIME(6)     NULL,
  revoked_at  DATETIME(6)     NULL,
  created_at  DATETIME(6)     NOT NULL,
  UNIQUE KEY uq_note_collections_public_id (public_id),
  UNIQUE KEY uq_note_collections_token_hash (token_hash),
  KEY ix_note_collections_yearbook (yearbook_id, created_at),
  CONSTRAINT fk_note_collections_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbooks (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose Down
DROP TABLE note_collections;
