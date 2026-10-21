-- +goose Up
-- T-111 (D-35): which version of the Terms and Privacy Policy a user accepted and when (Unix ms), and the version shown with a
-- friend's note. Expand-only: NULL means "never accepted" (accounts created before this change, new Google accounts), which the
-- API gate treats as "must accept"; the notes already stored stay NULL.
ALTER TABLE user_tab
  ADD COLUMN terms_version VARCHAR(16) NULL,
  ADD COLUMN terms_accepted_at BIGINT NULL;
ALTER TABLE note_tab ADD COLUMN terms_version VARCHAR(16) NULL;

-- +goose Down
ALTER TABLE note_tab DROP COLUMN terms_version;
ALTER TABLE user_tab DROP COLUMN terms_accepted_at, DROP COLUMN terms_version;
