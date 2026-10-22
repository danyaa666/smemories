-- +goose Up
-- T-095 (E11, L-14): an optional handwritten signature per note, stored as the canonical v1 JSON of vector strokes (internal/signature),
-- at most 3072 bytes. NULL = no signature. Expand-only: nullable, no default needed, so the previous build keeps working while this one
-- rolls out; ALGORITHM=INSTANT is a metadata-only change (no table rebuild, no long lock), and it fails loudly rather than rebuild.
ALTER TABLE note_tab ADD COLUMN signature_json VARCHAR(3072) CHARACTER SET ascii NULL DEFAULT NULL COMMENT 'canonical signature JSON (integers only), NULL = none', ALGORITHM=INSTANT;

-- +goose Down
ALTER TABLE note_tab DROP COLUMN signature_json;
