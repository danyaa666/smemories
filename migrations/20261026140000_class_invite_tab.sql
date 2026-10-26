-- +goose Up
-- T-125 (E07, D-36): join links of a class. Expand-only (one new table). No foreign keys (docs/db-conventions.md): the class
-- service deletes a class's invites with the class. The link token is a bearer secret and is never stored: token_hash is its
-- SHA-256, the only way to find an invite.
CREATE TABLE class_invite_tab (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  public_id  CHAR(26)        NOT NULL COMMENT 'ULID shown to the admin: how an invite is revoked (never the token)',
  class_id   BIGINT UNSIGNED NOT NULL COMMENT 'class_tab.id',
  token_hash BINARY(32)      NOT NULL COMMENT 'SHA-256 of the link token; the token itself is shown once and never stored',
  created_by BIGINT UNSIGNED NOT NULL COMMENT 'user_tab.id of the admin who made the link',
  expires_at BIGINT          NOT NULL COMMENT 'Unix ms',
  revoked_at BIGINT          NULL COMMENT 'Unix ms',
  created_at BIGINT          NOT NULL COMMENT 'Unix ms',
  updated_at BIGINT          NOT NULL COMMENT 'Unix ms',
  PRIMARY KEY (id),
  UNIQUE KEY uq_class_invite_tab_public_id (public_id),
  UNIQUE KEY uq_class_invite_tab_token_hash (token_hash),
  KEY idx_class_id (class_id),
  KEY idx_created_by (created_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE class_invite_tab;
