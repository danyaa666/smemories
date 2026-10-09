-- +goose Up
-- T-048 (D-22, D-23): email verification and password reset use 6-digit codes kept in Redis, so the link-token table of T-007 goes.
DROP TABLE email_tokens;

-- +goose Down
-- Recreates the empty table of 0005_email_tokens.sql.
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
