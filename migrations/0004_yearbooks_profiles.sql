-- +goose Up
-- Text columns are NOT NULL DEFAULT '' (empty = not set); graduation_year, birthday and
-- template_id are NULL until set. Deleting a user deletes their books, and a book its profiles.
CREATE TABLE yearbooks (
  id              BIGINT UNSIGNED   NOT NULL AUTO_INCREMENT PRIMARY KEY,
  public_id       CHAR(26)          NOT NULL,
  owner_id        BIGINT UNSIGNED   NOT NULL,
  title           VARCHAR(120)      NOT NULL,
  school_name     VARCHAR(120)      NOT NULL DEFAULT '',
  class_name      VARCHAR(120)      NOT NULL DEFAULT '',
  graduation_year SMALLINT UNSIGNED NULL,
  motto           VARCHAR(200)      NOT NULL DEFAULT '',
  language        ENUM('en','vi')   NOT NULL,
  page_size       ENUM('A5','A4')   NOT NULL DEFAULT 'A5',
  template_id     VARCHAR(32)       NULL COMMENT 'NULL until a template is chosen (T-010)',
  created_at      DATETIME(6)       NOT NULL,
  updated_at      DATETIME(6)       NOT NULL,
  UNIQUE KEY uq_yearbooks_public_id (public_id),
  KEY ix_yearbooks_owner_updated (owner_id, updated_at),
  CONSTRAINT fk_yearbooks_owner FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE profiles (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  public_id      CHAR(26)        NOT NULL,
  yearbook_id    BIGINT UNSIGNED NOT NULL,
  is_owner       BOOLEAN         NOT NULL,
  -- 1 for the owner's profile, NULL otherwise: the unique key then allows at most one owner profile per book.
  owner_flag     TINYINT GENERATED ALWAYS AS (IF(is_owner, 1, NULL)) STORED,
  full_name      VARCHAR(100)    NOT NULL,
  nickname       VARCHAR(50)     NOT NULL DEFAULT '',
  birthday       DATE            NULL,
  quote          VARCHAR(500)    NOT NULL DEFAULT '',
  hobbies        VARCHAR(300)    NOT NULL DEFAULT '',
  future_plans   VARCHAR(300)    NOT NULL DEFAULT '',
  photo_media_id BIGINT UNSIGNED NULL COMMENT 'NULL until T-009',
  UNIQUE KEY uq_profiles_public_id (public_id),
  UNIQUE KEY uq_profiles_one_owner (yearbook_id, owner_flag),
  CONSTRAINT fk_profiles_yearbook FOREIGN KEY (yearbook_id) REFERENCES yearbooks (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose Down
DROP TABLE profiles;
DROP TABLE yearbooks;
