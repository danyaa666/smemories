-- +goose Up
-- T-052 (D-23): login sessions live in Redis with native expiry, so the sessions table goes. Nothing is in production:
-- there is no data to move, and every user simply signs in again.
DROP TABLE sessions;

-- +goose Down
-- Recreates the empty table of 0002_users_sessions.sql.
CREATE TABLE sessions (
  token_hash   BINARY(32)      NOT NULL PRIMARY KEY COMMENT 'sha256 of the cookie value',
  user_id      BIGINT UNSIGNED NOT NULL,
  created_at   DATETIME(6)     NOT NULL,
  last_seen_at DATETIME(6)     NOT NULL,
  expires_at   DATETIME(6)     NOT NULL,
  user_agent   VARCHAR(255)    NOT NULL DEFAULT '',
  KEY ix_sessions_user (user_id, expires_at),
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
