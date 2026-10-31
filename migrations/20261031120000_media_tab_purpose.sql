-- +goose Up
-- T-152 (E09, D-43): what an image is for. 'photo' is every yearbook photo (all existing rows, so the default), 'design' is an image
-- a student uploads as a background or decoration of the layout editor. VARCHAR, no ENUM (docs/db-conventions.md); the values are
-- validated in Go. Expand-only: the column has a default, so the previous build keeps working while this one rolls out (it would
-- only list design images as photos, which is why no design row can exist before this build is live). No new index: every query
-- is already scoped by yearbook_id (idx_yearbook_id), at most 200 + 30 rows per yearbook.
ALTER TABLE media_tab
  ADD COLUMN purpose VARCHAR(16) NOT NULL DEFAULT 'photo' COMMENT 'photo | design (validated in Go)';

-- +goose Down
-- Design images do not exist in the previous schema, and without the column the old build would show them in the photo gallery and
-- count them as photos. Their rows go; the stored objects are removed with the yearbook's prefix when the book is deleted.
DELETE FROM media_tab WHERE purpose = 'design';
ALTER TABLE media_tab DROP COLUMN purpose;
