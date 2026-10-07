-- +goose Up
-- One row per emailed link. Only the SHA-256 of the token is stored; a row is single use
-- (used_at) and short lived (expires_at). Rows go away with their user.
CREATE TABLE email_tokens (
  token_hash BINARY(32)      NOT NULL PRIMARY KEY COMMENT 'sha256 of the token in the link',
  user_id    BIGINT UNSIGNED NOT NULL,
  purpose    ENUM('verify','reset') NOT NULL,
  expires_at DATETIME(6)     NOT NULL,
  used_at    DATETIME(6)     NULL,
  created_at DATETIME(6)     NOT NULL,
  KEY ix_email_tokens_user (user_id, purpose),
  KEY ix_email_tokens_expires (expires_at),
  CONSTRAINT fk_email_tokens_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose Down
DROP TABLE email_tokens;
