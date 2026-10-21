-- +goose Up
-- T-092 (D-32): the epoch of the public photo links of a book. A link token carries a MAC over (media id, book id, link_epoch);
-- POST /api/yearbook/revoke-photo-links adds 1, so every link made before stops verifying at once. Expand-only: the column has a
-- default, so the previous build keeps working while this one rolls out. No index: it is read together with the book row.
ALTER TABLE yearbook_tab ADD COLUMN link_epoch INT UNSIGNED NOT NULL DEFAULT 0 COMMENT 'public photo link epoch, +1 per revoke-all';

-- +goose Down
ALTER TABLE yearbook_tab DROP COLUMN link_epoch;
