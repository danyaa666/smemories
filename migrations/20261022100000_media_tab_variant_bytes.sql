-- +goose Up
-- T-105: the quotas count every stored variant of a photo (display, thumb, print), so the thumbnail and print sizes are kept next to
-- `bytes` (the display size). Expand-only: the columns have a default, so the previous build keeps working while this one rolls out.
-- Rows written before this migration stay 0 (no production data, so no backfill; the print backfill fills print_bytes for its rows).
ALTER TABLE media_tab
  ADD COLUMN thumb_bytes INT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'size of the thumb object, 0 = not recorded',
  ADD COLUMN print_bytes INT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'size of the print object, 0 = none or not recorded';

-- +goose Down
ALTER TABLE media_tab DROP COLUMN thumb_bytes, DROP COLUMN print_bytes;
