-- +goose Up
-- T-141 (E09, D-41): one customisation per yearbook (theme, fonts, section order, texts, images), chosen from the choices the html
-- template declares in its manifest "design" block. Expand-only (one new table). No foreign keys (docs/db-conventions.md): the
-- yearbook service deletes the row with the yearbook (ChildPurger) and when the template changes. options_json is JSON text
-- (version 1) validated in Go against the manifest before every write, never more than 4096 bytes; it is plain data, never interpreted
-- as markup. template_id is the template the options were made for.
CREATE TABLE book_design_tab (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  yearbook_id  BIGINT UNSIGNED NOT NULL COMMENT 'yearbook_tab.id, one row per yearbook',
  template_id  VARCHAR(32)     NOT NULL COMMENT 'template the options were made for',
  options_json VARCHAR(4096)   NOT NULL COMMENT 'options document v1, at most 4096 bytes (validated in Go)',
  created_at   BIGINT          NOT NULL COMMENT 'Unix ms',
  updated_at   BIGINT          NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_book_design_tab_yearbook_id (yearbook_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE book_design_tab;
