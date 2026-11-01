# Version history in the editor (T-165)

The layout editor keeps the document a student replaced as a **snapshot** on the server (T-163, [templates.md](templates.md), "Version history"): the newest 30 per book, one automatic snapshot per editing burst (the newest one must be older than 10 minutes), plus one before every reset and every restore. The **History** button in the toolbar opens a panel that lists them, previews one and restores one.

## What the student sees

- **History** opens the panel in the place of the property panel. Nothing is requested before it opens; opening calls `version-list` once (never polled; **Refresh** calls it again). Up to 30 rows, newest first: relative time ("12 minutes ago", title = the exact time), kind (Autosave, Before reset, Before restore) and the item count. Empty, loading, error with *Try again* and offline each have a text.
- **Preview**: choosing a row calls `version-get` (once per row; the last 5 bodies stay in memory while the panel is open) and draws it on the stage with the book renderer, read-only: no handles, no gestures, the property panel, the toolbar edit buttons and the palette are disabled, undo and redo shortcuts do nothing, and the preview never saves. The page strip still works. The bar in the panel says "Previewing a version from <time>. Your current work is unchanged." and counts the items whose sticker or design image is gone ("N items will be dropped": a hint from the asset registry and the loaded design image list; the server's `dropped` is the truth; it counts no image while the list of design images is partial). **Back to current** (or Esc) leaves the preview; closing the panel does too.
- **Restore this version** asks for confirmation, then: `settle()` (the pending and in-flight saves are finished, so the server's version is the editor's), `restore` with `base_version` = the store's version, and the editor adopts the answer. Undo and redo are **cleared**, because the restored document does not descend from the old steps. The replaced document is kept by the server as a "Before restore" row, so nothing is lost. "Version restored." and, when `dropped > 0`, "N items could not be restored because their image or sticker no longer exists" appear in the live region and in a toast. A double click sends one request.
- Errors, each with its own vi and en text: another tab saved (`ERROR_VERSION_CONFLICT`: the editor's conflict banner takes over with the stored document, nothing is overwritten), the version no longer fits the template (`ERROR_INVALID_LAYOUT`) or was made for another template (`ERROR_VERSION_TEMPLATE_CHANGED`) (both: *Back to current*), 404 (pruned meanwhile: the list is read again), 429 (the Retry-After seconds), offline, and "your latest changes could not be saved yet" when the settle step fails (nothing is sent). A failed restore leaves the document, its history and the autosave as they were.

The three routes share the layout save rate limit (burst 20 per 10 s, 60 a minute), which is why nothing is polled and previews are cached.

## Keyboard

History button (Enter), Tab to the list (a radio group: arrows, Home, End move, Enter or Space previews), Shift+Tab from the list reaches **Restore this version** and **Back to current** (the bar is above the list in the panel), Enter confirms in the dialog, Esc leaves the preview, Esc again closes the panel and puts focus on the History button.

## Where it is

`web/src/book/editor/versions.ts` (relative time, missing-id hint, preview cache, restore flow with a fake-able API), `HistoryPanel.tsx`, `EditorShell.tsx`, `store.ts` (`adopt`, `enterConflict`, `currentVersion`). Tests: `versions.test.ts`, `EditorHistory.test.tsx`, `tools/e2e/editor-history.spec.mjs`.

## E2E and snapshots

A save makes a snapshot only after 10 minutes. The Playwright spec cannot wait, so it moves the stored snapshots 11 minutes into the past with the `mysql` command line client against `SMEM_DB_DSN` (the throwaway test database that `guard.mjs` already checks; the client must be installed: the GitHub runner and `brew install mysql-client` have it). The server clock is untouched.

## Check by hand (not proven by a machine)

- **Phone widths**: the editor does not mount below 768 px (a message with a link back shows), so there is no phone history dialog; check that message on a real phone. On a tablet at 768 to 1100 px check that the panel, the stage and the dashed preview outline fit without horizontal scroll.
- Screen readers: the live region texts ("Previewing...", "Version restored.", the dropped message) and the radio group names.
- Two real devices restoring at once.
