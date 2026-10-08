-- +goose Up
-- External sign-in identities (Google). (provider, subject) is the stable key; the email
-- is what the provider reported at link time and is informational only.
CREATE TABLE user_identities (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id    BIGINT UNSIGNED NOT NULL,
  provider   ENUM('google')  NOT NULL,
  subject    VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  email      VARCHAR(254)    NOT NULL,
  created_at DATETIME(6)     NOT NULL,
  UNIQUE KEY uq_identity_subject (provider, subject),
  KEY ix_identity_user (user_id),
  CONSTRAINT fk_identity_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose Down
DROP TABLE user_identities;
