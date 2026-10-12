-- +goose Up
-- T-067 (docs/db-conventions.md): users -> user_tab, user_identities -> user_identity_tab. user_identity_tab loses the last
-- foreign key of the schema (the auth service deletes identities with the user, see T-025); locale and provider are VARCHAR
-- instead of ENUM (validated in Go); email_verified_at, created_at and updated_at are BIGINT Unix ms; user_identity_tab gets
-- updated_at (= created_at for existing rows). The email column is not touched: it stays utf8mb4_bin (accents and case count).
-- The old DATETIME(6) values are UTC; TIMESTAMPDIFF does not depend on the session time zone. Sub-millisecond digits are
-- dropped (a negative value truncates towards zero); a NULL email_verified_at stays NULL.
RENAME TABLE users TO user_tab, user_identities TO user_identity_tab;
ALTER TABLE user_identity_tab DROP FOREIGN KEY fk_identity_user;

ALTER TABLE user_tab
  RENAME INDEX uq_users_public_id TO uq_user_tab_public_id,
  RENAME INDEX uq_users_email TO uq_user_tab_email,
  MODIFY locale VARCHAR(8) NOT NULL DEFAULT 'en',
  ADD COLUMN email_verified_at_ms BIGINT NULL,
  ADD COLUMN created_at_ms BIGINT NULL,
  ADD COLUMN updated_at_ms BIGINT NULL;
UPDATE user_tab SET
  email_verified_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', email_verified_at) DIV 1000,
  created_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', created_at) DIV 1000,
  updated_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', updated_at) DIV 1000;
ALTER TABLE user_tab DROP COLUMN email_verified_at, DROP COLUMN created_at, DROP COLUMN updated_at;
ALTER TABLE user_tab
  RENAME COLUMN email_verified_at_ms TO email_verified_at,
  RENAME COLUMN created_at_ms TO created_at,
  RENAME COLUMN updated_at_ms TO updated_at;
ALTER TABLE user_tab MODIFY created_at BIGINT NOT NULL, MODIFY updated_at BIGINT NOT NULL;

ALTER TABLE user_identity_tab
  RENAME INDEX uq_identity_subject TO uq_user_identity_provider_subject,
  RENAME INDEX ix_identity_user TO idx_user_id,
  MODIFY provider VARCHAR(16) NOT NULL,
  ADD COLUMN created_at_ms BIGINT NULL;
UPDATE user_identity_tab SET created_at_ms = TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', created_at) DIV 1000;
ALTER TABLE user_identity_tab DROP COLUMN created_at;
ALTER TABLE user_identity_tab RENAME COLUMN created_at_ms TO created_at;
ALTER TABLE user_identity_tab MODIFY created_at BIGINT NOT NULL, ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;
UPDATE user_identity_tab SET updated_at = created_at;
ALTER TABLE user_identity_tab ALTER COLUMN updated_at DROP DEFAULT;

-- +goose Down
-- Restores the schema and the values (milliseconds back to DATETIME(6)). Identities of a missing user, which only the foreign
-- key would have prevented, are deleted first. Locale and provider values outside the old enums cannot exist (the API
-- validates them), so the narrowing back to ENUM does not lose rows.
DELETE FROM user_identity_tab WHERE user_id NOT IN (SELECT id FROM user_tab);
ALTER TABLE user_identity_tab
  DROP COLUMN updated_at,
  ADD COLUMN created_at_dt DATETIME(6) NULL;
UPDATE user_identity_tab SET created_at_dt = TIMESTAMPADD(MICROSECOND, created_at * 1000, '1970-01-01 00:00:00');
ALTER TABLE user_identity_tab DROP COLUMN created_at;
ALTER TABLE user_identity_tab RENAME COLUMN created_at_dt TO created_at;
ALTER TABLE user_identity_tab
  MODIFY created_at DATETIME(6) NOT NULL,
  MODIFY provider ENUM('google') NOT NULL,
  RENAME INDEX idx_user_id TO ix_identity_user,
  RENAME INDEX uq_user_identity_provider_subject TO uq_identity_subject,
  ADD CONSTRAINT fk_identity_user FOREIGN KEY (user_id) REFERENCES user_tab (id) ON DELETE CASCADE;

ALTER TABLE user_tab
  ADD COLUMN email_verified_at_dt DATETIME(6) NULL,
  ADD COLUMN created_at_dt DATETIME(6) NULL,
  ADD COLUMN updated_at_dt DATETIME(6) NULL;
UPDATE user_tab SET
  email_verified_at_dt = TIMESTAMPADD(MICROSECOND, email_verified_at * 1000, '1970-01-01 00:00:00'),
  created_at_dt = TIMESTAMPADD(MICROSECOND, created_at * 1000, '1970-01-01 00:00:00'),
  updated_at_dt = TIMESTAMPADD(MICROSECOND, updated_at * 1000, '1970-01-01 00:00:00');
ALTER TABLE user_tab DROP COLUMN email_verified_at, DROP COLUMN created_at, DROP COLUMN updated_at;
ALTER TABLE user_tab
  RENAME COLUMN email_verified_at_dt TO email_verified_at,
  RENAME COLUMN created_at_dt TO created_at,
  RENAME COLUMN updated_at_dt TO updated_at;
ALTER TABLE user_tab
  MODIFY created_at DATETIME(6) NOT NULL,
  MODIFY updated_at DATETIME(6) NOT NULL,
  MODIFY locale ENUM('en','vi') NOT NULL DEFAULT 'en',
  RENAME INDEX uq_user_tab_public_id TO uq_users_public_id,
  RENAME INDEX uq_user_tab_email TO uq_users_email;
RENAME TABLE user_tab TO users, user_identity_tab TO user_identities;
