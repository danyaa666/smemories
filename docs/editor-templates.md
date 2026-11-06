# Page templates in the editor (T-175, T-176)

The **Templates** button in the editor's toolbar (vi "Mẫu trang") opens a side panel that lists the templates of **the kind of the page being edited** and applies one to that kind. It is the screen over the server side of T-172 (shipped templates, `template-list`) and T-173 (own templates, `template-create`, `template-delete`, `apply-template`); the rules of those routes are in [api-contract.md](api-contract.md) and [templates.md](templates.md).

## What it does, per section

- One panel for the four kinds (cover, profile, notes, back). Its title names the kind ("Templates for the cover"). When the student steps to a page of another kind with the panel open, the list follows; stepping between two notes pages keeps it, because **a notes template applies to every notes page** (a line says so, the same sharing the T-170 notes line states).
- Two groups: **Shipped** (the book's template's own files, in the order the server sorts them, named in the app language) and **My templates** (the student's, newest first, named as the student named them). Each card has a thumbnail, the name, the item count, **Preview** (the card itself) and **Apply**; an own card has **Delete**. A shipped card never has Delete.
- The list is read **only when the panel opens** (never when the editor loads), again when the kind changes (the lists of the two newest kinds are kept while the panel is open), on **Refresh**, and after a template error that makes the list stale. It is never polled.
- Thumbnails are the template's page drawn by the book's own page components on the book's real data (names, photos), read-only, scaled by CSS, never an image from the server. Only the cards near the viewport draw one, and never more than 12 at once; the others show a grey placeholder. An own template has no images (the server does not copy `image` items and picture backgrounds), so its card says "Images not included" and its thumbnail shows text, stickers and shapes only.

## Preview against apply

- **Preview** shows the current layout with `pages.<kind>` replaced by the template's page (`withPage` in `document.ts`), in the stage, read-only like the History preview: the toolbar, the gestures and the keys of the editor are off, nothing is sent, autosave does not run for the preview. A banner says "Previewing the template 'X' on the cover. Your page is unchanged." with *Apply to the cover* and *Back to current*, and counts the items whose sticker or image the client knows is gone ("N items will be left out": a hint, the server's `dropped` is the truth). The page strip works for pages of the same kind only; stepping to another kind ends the preview.
- **Apply** (from a card or from the banner) asks one confirmation that names the section and the consequence, unless the page has no moved element, item or background (nothing to lose). Then: `store.settle()` (every save in flight and queued is finished, so a pending edit is in the snapshot and the server's version is the editor's), `apply-template` with `base_version` read after the settle, and on success the editor **adopts** the answer (`layout`, `version`), leaves the preview and closes the panel with the focus on the Templates button. A double click sends one request. The live region says "Template applied to the cover." (plus how many items could not be applied) and a toast offers **Undo in History**.
- **Why undo and redo are cleared, and where undo lives.** The answer is a document the server wrote, not a descendant of the editor's undo steps, so the steps go (the confirmation says so), as after a restore. The way back is the History panel: the document the template replaced is one snapshot of kind `apply`, labelled "Before template" (vi "Trước khi áp dụng mẫu"); restoring it brings the old page back. A snapshot kind the web does not know yet shows as "Earlier version".

## Own and shipped

Shipped templates come from files of the server and cannot be changed or deleted. Own templates are created from a page the student saved (see "Saving a page as a template" below), at most 20 per user across all page kinds, are text, stickers and shapes only, and belong to the book template they were made on. **Delete** asks "Delete the template 'X'? Books that already used it are not changed.", removes the card and announces it; a 404 (already gone) just refreshes the list.

## Saving a page as a template (T-176)

**Save this page as a template** (vi "Lưu trang này thành mẫu") is in the panel next to Refresh, for the kind being edited; there is no second toolbar button, so the keyboard reaches it through Templates, Tab. It opens a name dialog and sends `template-create {yearbook_id, kind, name}`.

- **When it is offered.** It explains instead of opening a dialog (the button is `aria-disabled`, so it stays focusable, and the reason is under it) when the page has no moved element, item or background ("Nothing to save: this page has no changes yet"), while the browser is offline, while a template is previewed (the stage then shows the template, not the stored page), and after a 409 limit answer until a template is deleted or the list is refreshed.
- **The dialog.** A name (required, plain text, trimmed, at most 60 code points; the counter is a polite live region; default "My cover 2" or "Bìa của tôi 2" from the own templates of the kind already listed), a summary "This saves the cover as it is now: 5 items, 1 moved element, a background. N images are not saved (your own images are not copied to templates)" and *Save* / *Cancel* (*Back* after `ERROR_INVALID_LAYOUT`). Enter saves (not while an IME composes), Esc cancels and returns the focus to the button, Tab stays inside. The name rules are the server's `textx.Clean`: NFC, then trim, then 1 to 60 code points, no control, invisible or line-separator characters (a joiner is allowed); composed and decomposed Vietnamese count the same, because they are compared after NFC, and the name is sent composed. An empty or too long name shows an inline error and sends nothing. The image count in the summary is a **hint from the editor's document** (`image` items and a picture `bg.media`, exactly what the server strips); the server's `stripped` is the truth and is what the toast says.
- **Save.** `store.settle()` first (the server copies the book's **stored** page, so a pending edit must be saved before the copy), then one `template-create` (a double click sends one request). Success closes the dialog, announces and shows a toast "Template 'X' saved." plus "N images were not saved." when `stripped > 0`, reads the list of the kind again and puts the focus on the new card, which is announced and shown as "new". **A save changes nothing in the editor**: no `adopt`, the document, version, undo stack and autosave are exactly as before, and no `save` is sent for it.
- **The cap of 20 is per account, across all page kinds**, while the panel lists one kind of one book template at a time, and `template-list` returns no total. So the panel does **not** show a count like "3 of 20" (it would be wrong): the own group says "Up to 20 own templates per account, across all page types", and the 409 (`data.limit`) message says "You have 20 own templates, the most you can keep across all page types. Delete one in the panel first." The dialog stays open with the typed name and the own group is scrolled into view; afterwards the button shows that message instead of opening a dialog that is bound to fail, until a template is deleted in the panel (or Refresh).

### Save errors (`editor.templates.save.err.*`, vi and en)

Nothing in the editor changes for any of them, the dialog stays open and the typed name is kept.

| Case | What the student sees |
| --- | --- |
| `ERROR_TEMPLATE_LIMIT` (409, `data.limit`) | the limit message above |
| `ERROR_TOO_LARGE` | "This page is too big to keep as a template. Remove some items and try again." |
| `ERROR_INVALID_LAYOUT` | a generic message (the problem list is not shown) and *Back* |
| `ERROR_PARAM` | "The name was not accepted, or this page has nothing saved yet." |
| 404 | "This book no longer exists" |
| 429 | "Try again in N s" (the Retry-After) |
| offline | "You are offline" |
| the settle step failed | "latest changes could not be saved yet"; nothing is sent |

The round trip: the template shows in "My templates" of the same book template and kind, previews and applies like any other (also after the page was reset), and is not listed for another kind or for a book on another template (the server filters by book template and kind).

## Errors of apply (each with its own vi and en text, `editor.templates.err.*`)

A failed apply leaves the editor, its history and its autosave exactly as before.

| Case | What the student sees |
| --- | --- |
| `ERROR_VERSION_CONFLICT` (another tab saved) | The editor's conflict banner with the stored document; nothing is overwritten. |
| `ERROR_INVALID_LAYOUT` | "no longer fits the book's design"; the preview stays so the student can *Back to current* or *Refresh*. |
| `ERROR_TEMPLATE_MISMATCH` (409) | "made for another design of the book"; the preview ends, the list is read again. |
| 404 | "no longer exists"; the preview ends, the list is read again. |
| 429 | "Try again in N s" (the Retry-After). |
| offline | "You are offline". |
| the settle step failed | "latest changes could not be saved yet"; nothing is sent. |

## Limits and cost

No `template-list` call before the panel opens; at most two kinds in memory; `apply-template` is the only write and is never sent by a preview; the three template routes share the save rate limit (burst 20 per 10 s, 60 a minute). The panel is only offered for a book template that has a `design.layout` block (the editor itself is not mounted otherwise). The editor does not mount below 768 px, so the panel has no phone-width dialog form; below that width the route shows its own message.

## Tests

`templates.test.ts` (list model, `withPage`, apply flow with a fake API, and for T-176 the name rules, the image hint, the save error mapping and the save state machine), `EditorTemplates.test.tsx` (panel and shell: keyboard walk, preview, apply once, errors, delete), `EditorSaveTemplate.test.tsx` (the button and its reasons, the dialog's keyboard flow, settle first, one request on a double click, the list read again with the new card focused, the editor untouched, every error, the 21st template), the label cases in `EditorHistory.test.tsx`, and `tools/e2e/editor-templates.spec.mjs` and `tools/e2e/editor-save-template.spec.mjs` ([e2e.md](e2e.md)).

## Not proven by a machine (check by hand)

- A screen reader reading the card names ("Template Clean title, 6 items, shipped"), the banner and the live region.
- Touch: the 44 px targets of the cards' buttons on a tablet (the editor needs at least 768 px) and a drag-free page scroll over the list.
- A shipped template on each book template and page kind drawn as the thumbnail shows it (the thumbnail and the applied page use the same components; the print check covers the page itself).
- The save dialog read by a screen reader (the summary, the polite counter, the error), and the name typed on a phone-sized touch device with Telex or VNI (the e2e types composed text only).
