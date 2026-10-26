-- +goose Up
-- T-127 (E07, D-36): class-level pages of the class book (intro, teachers, events, ...). Expand-only (one new table). No foreign
-- keys (docs/db-conventions.md): the class service deletes a class's pages with the class. kind and placement are validated in Go
-- (no ENUM): kind is intro|teachers|events|superlatives|custom, placement is front|back. body is plain text: stored and returned
-- as typed, never interpreted as HTML.
CREATE TABLE class_page_tab (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  public_id  CHAR(26)        NOT NULL COMMENT 'ULID shown to clients',
  class_id   BIGINT UNSIGNED NOT NULL COMMENT 'class_tab.id',
  kind       VARCHAR(16)     NOT NULL COMMENT 'intro, teachers, events, superlatives or custom',
  title      VARCHAR(120)    NOT NULL,
  body       TEXT            NOT NULL COMMENT 'plain text, at most 4000 characters (validated in Go), newlines kept',
  placement  VARCHAR(8)      NOT NULL COMMENT 'front (before the students) or back (after them)',
  sort_order INT             NOT NULL DEFAULT 0 COMMENT 'position within the placement',
  created_at BIGINT          NOT NULL COMMENT 'Unix ms',
  updated_at BIGINT          NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_class_page_tab_public_id (public_id),
  KEY idx_class_id (class_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE class_page_tab;
