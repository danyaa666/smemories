-- +goose Up
-- T-153 (E09 stage 2, D-42, ADR 0004): one free-layout document per yearbook (moved or hidden template elements, added text boxes,
-- stickers, shapes and images, page backgrounds), validated in Go against the template manifest's design.layout block before every
-- write. Expand-only (one new table). No foreign keys (docs/db-conventions.md): the yearbook service deletes the row with the yearbook
-- (ChildPurger) and when the template changes. layout_json is JSON text (document v1, at most 131072 bytes, enforced in Go; MEDIUMTEXT
-- because a row cannot hold 128 KiB in a VARCHAR), plain data, never interpreted as markup. version starts at 1 and grows by 1 per
-- save; a save names the version it read (optimistic concurrency, UPDATE ... WHERE version = ?). template_id is the template the
-- document was made for.
CREATE TABLE book_layout_tab (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  yearbook_id BIGINT UNSIGNED NOT NULL COMMENT 'yearbook_tab.id, one row per yearbook',
  template_id VARCHAR(32)     NOT NULL COMMENT 'template the layout was made for',
  layout_json MEDIUMTEXT      NOT NULL COMMENT 'layout document v1, at most 131072 bytes (validated in Go)',
  version     BIGINT          NOT NULL COMMENT 'starts at 1, +1 per save',
  created_at  BIGINT          NOT NULL COMMENT 'Unix ms',
  updated_at  BIGINT          NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_book_layout_tab_yearbook_id (yearbook_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE book_layout_tab;
