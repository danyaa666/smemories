-- +goose Up
-- US Letter (215.9 x 279.4 mm) as a third yearbook page size (T-037). Additive: existing rows are untouched.
ALTER TABLE yearbooks MODIFY page_size ENUM('A5','A4','Letter') NOT NULL DEFAULT 'A5';

-- +goose Down
-- A Letter book cannot exist in the old enum, so it falls back to the default size.
UPDATE yearbooks SET page_size = 'A5' WHERE page_size = 'Letter';
ALTER TABLE yearbooks MODIFY page_size ENUM('A5','A4') NOT NULL DEFAULT 'A5';
