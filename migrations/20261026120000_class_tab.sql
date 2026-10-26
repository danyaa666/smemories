-- +goose Up
-- T-026 (E07, D-36): the class yearbook. A class is a thin layer over the existing personal yearbooks: class_tab is the space,
-- class_member_tab says who is in it (role admin or student) and which of their own yearbooks is their class page.
-- Expand-only (two new tables, nothing existing is touched). No foreign keys (docs/db-conventions.md): the class service
-- deletes the members with the class, and clears yearbook_id when the yearbook is deleted (yearbook.ChildPurger).
-- Values are validated in Go (no ENUM): language is en|vi, role is admin|student.
CREATE TABLE class_tab (
  id              BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
  public_id       CHAR(26)         NOT NULL COMMENT 'ULID shown to clients',
  name            VARCHAR(120)     NOT NULL,
  school_name     VARCHAR(120)     NOT NULL DEFAULT '',
  graduation_year SMALLINT         NULL,
  language        VARCHAR(8)       NOT NULL,
  created_by      BIGINT UNSIGNED  NOT NULL COMMENT 'user_tab.id of the creator',
  created_at      BIGINT           NOT NULL COMMENT 'Unix ms',
  updated_at      BIGINT           NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_class_tab_public_id (public_id),
  KEY idx_created_by (created_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE class_member_tab (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  public_id   CHAR(26)        NOT NULL COMMENT 'ULID shown to clients: how an admin names a member (the user id never leaves)',
  class_id    BIGINT UNSIGNED NOT NULL COMMENT 'class_tab.id',
  user_id     BIGINT UNSIGNED NOT NULL COMMENT 'user_tab.id',
  role        VARCHAR(16)     NOT NULL COMMENT 'admin or student',
  yearbook_id BIGINT UNSIGNED NULL COMMENT 'yearbook_tab.id of the member''s class page; one yearbook belongs to at most one class',
  sort_order  INT             NOT NULL DEFAULT 0 COMMENT 'position in the class book',
  joined_at   BIGINT          NOT NULL COMMENT 'Unix ms',
  created_at  BIGINT          NOT NULL COMMENT 'Unix ms',
  updated_at  BIGINT          NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_class_member_tab_public_id (public_id),
  UNIQUE KEY uq_class_member_tab_class_user (class_id, user_id),
  UNIQUE KEY uq_class_member_tab_yearbook (yearbook_id), -- NULLs do not collide
  KEY idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE class_member_tab;
DROP TABLE class_tab;
