-- +goose Up
-- email is utf8mb4_bin: the app lower-cases it, and the default accent-insensitive
-- collation would treat "é" and "e" as the same address.
CREATE TABLE users (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  public_id         CHAR(26)     NOT NULL,
  email             VARCHAR(254) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  email_verified_at DATETIME(6)  NULL,
  password_hash     VARCHAR(255) NULL COMMENT 'argon2id PHC string; NULL for social-only accounts',
  display_name      VARCHAR(100) NOT NULL,
  locale            ENUM('en','vi') NOT NULL DEFAULT 'en',
  created_at        DATETIME(6)  NOT NULL,
  updated_at        DATETIME(6)  NOT NULL,
  UNIQUE KEY uq_users_public_id (public_id),
  UNIQUE KEY uq_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

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

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
