-- +goose Up
-- T-156 (E09): the layout version of a yearbook never goes back. book-layout/reset and a template change delete the book_layout_tab
-- row, so the version kept on that row restarted at 1 and a stale editor could win an ABA (X holds 3, reset, three saves elsewhere,
-- X saves with base_version 3 and overwrites). The counter now lives on the yearbook row, which reset and template change do not
-- delete: it is the last version issued (0 = never saved), +1 per save, +1 per delete of the layout. Expand-only: one new column,
-- backfilled from the existing layout rows so stored versions are preserved; book_layout_tab.version stays and mirrors it.
ALTER TABLE yearbook_tab ADD COLUMN layout_version BIGINT NOT NULL DEFAULT 0 COMMENT 'last layout version issued, never decreases (0 = none yet)';
UPDATE yearbook_tab y JOIN book_layout_tab l ON l.yearbook_id = y.id SET y.layout_version = l.version;

-- +goose Down
ALTER TABLE yearbook_tab DROP COLUMN layout_version;
