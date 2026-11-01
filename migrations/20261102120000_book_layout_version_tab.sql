-- +goose Up
-- T-163 (E09, D-43): the history of a book's layout. A snapshot is the document as it was before a save that started a new editing burst
-- (kind auto), before a reset (reset) or before a restore (restore); the service keeps the newest 30 per book and writes and prunes them in
-- the same transaction as the write that replaced the document (under the yearbook row lock). Expand-only (one new table). No foreign keys
-- (docs/db-conventions.md): the layout service deletes the rows with the layout on a template change and with the yearbook (ChildPurger).
-- version is the layout version the replaced document held (unique per book: a version is replaced once). public_id is the ULID clients
-- address; a snapshot is never updated, so there is no updated_at (documented exception to the timestamp rule); item_count and bytes are kept so the list needs no document bodies. layout_json is the document v1 (at most 131072 bytes).
CREATE TABLE book_layout_version_tab (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  public_id   CHAR(26)        NOT NULL COMMENT 'ULID the client addresses',
  yearbook_id BIGINT UNSIGNED NOT NULL COMMENT 'yearbook_tab.id',
  version     BIGINT          NOT NULL COMMENT 'layout version the snapshot held when it was replaced',
  kind        VARCHAR(16)     NOT NULL COMMENT 'auto, reset or restore (validated in Go)',
  template_id VARCHAR(32)     NOT NULL COMMENT 'template the layout was made for',
  layout_json MEDIUMTEXT      NOT NULL COMMENT 'layout document v1, at most 131072 bytes',
  item_count  INT             NOT NULL COMMENT 'items on all pages, for the list',
  bytes       INT             NOT NULL COMMENT 'length of layout_json, for the list',
  created_at  BIGINT          NOT NULL COMMENT 'Unix ms, when the document was replaced',
  PRIMARY KEY (id),
  UNIQUE KEY uq_book_layout_version_tab_public_id (public_id),
  UNIQUE KEY uq_book_layout_version_tab_yearbook_id_version (yearbook_id, version),
  KEY idx_book_layout_version_tab_yearbook_id_created_at (yearbook_id, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE book_layout_version_tab;
