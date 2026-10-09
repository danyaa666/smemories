-- +goose Up
-- T-057: third stored size. NULL until the photo has a print object (existing rows are filled by cmd/smemories-media-backfill).
ALTER TABLE media ADD COLUMN print_key VARCHAR(255) NULL COMMENT 'object key of the 1800 px print version' AFTER thumb_key;

-- +goose Down
ALTER TABLE media DROP COLUMN print_key;
