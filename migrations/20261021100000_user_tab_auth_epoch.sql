-- +goose Up
-- T-103: credential epoch. A password reset or the Google pre-hijacking merge adds 1; a session stores the epoch it was issued under
-- (read in the same SELECT as the password hash) and Authenticate refuses a session whose epoch differs, so a login that raced the
-- credential change cannot leave a session behind. Expand-only: existing rows and sessions are epoch 0.
ALTER TABLE user_tab ADD COLUMN auth_epoch INT UNSIGNED NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_tab DROP COLUMN auth_epoch;
