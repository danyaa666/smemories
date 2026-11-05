-- +goose Up
-- T-173 (E09, ADR 0005 Q3): a student's own page templates. A template is the value of pages.<kind> of a layout document (bg, overrides,
-- items) with a name, copied from the owner's book with image items and picture backgrounds stripped (own images are not shareable). Owner
-- only: every query is scoped by owner_id; at most 20 rows per owner (the service counts under the user row lock) and 64 KiB each.
-- Expand-only (one new table). No foreign keys (docs/db-conventions.md): owner_id is user_tab.id and the account deletion (T-025) deletes
-- the rows with the user. template_id is the book template the page was made on (a template is applied only to a book that still has it).
-- A template is never updated, so there is no updated_at (documented exception to the timestamp rule, like book_layout_version_tab).
CREATE TABLE book_template_tab (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  public_id   CHAR(26)        NOT NULL COMMENT 'ULID the client addresses',
  owner_id    BIGINT UNSIGNED NOT NULL COMMENT 'user_tab.id',
  name        VARCHAR(60)     NOT NULL COMMENT 'plain text, 1 to 60 characters',
  kind        VARCHAR(16)     NOT NULL COMMENT 'page kind of the template block (validated in Go)',
  template_id VARCHAR(32)     NOT NULL COMMENT 'book template the page was made on',
  layout_json MEDIUMTEXT      NOT NULL COMMENT 'the page value: bg, overrides, items; at most 65536 bytes',
  item_count  INT             NOT NULL COMMENT 'items on the page, for the list',
  bytes       INT             NOT NULL COMMENT 'length of layout_json',
  created_at  BIGINT          NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_book_template_tab_owner_id_public_id (owner_id, public_id),
  KEY idx_book_template_tab_owner_id_created_at (owner_id, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE book_template_tab;
