# Palette, images and backgrounds in the editor (T-162)

The layout editor (`/yearbooks/:id/editor`, ADR 0004) has a fourth column, **Add to the page**, next to the property panel. It takes 280 px from the stage, so it is open at first only on a screen 1700 px wide or more; the toolbar button *Add to page* (`aria-expanded`) opens and closes it (the choice is not kept over a reload). On a screen narrower than 1100 px it goes under the stage.

## The tabs

| Tab | What it does |
|---|---|
| Stickers, Shapes, Frames | The assets of the registry (`web/src/book/assets/registry.json`, docs/assets.md) of that kind, as a grid of thumbnails. The label is the asset's `label_vi` or `label_en` by the language of the screen. The search ignores case and diacritics and looks in both labels and the tags (`nang` finds `nắng`, `dong` finds `Đồng`). The grid is one tab stop: arrow keys, Home and End move, Enter or click adds. A thumbnail draws only when it is near the screen. |
| My images | The book's design images (`media/get-list` with `purpose=design`, `thumb` variant, 24 per page and "Load more"), an *Upload image* button and a drop zone (JPEG, PNG, WebP; the client checks the type and 10 MiB before it sends anything, the server stays the judge). Picking one adds an `image` item. |
| Background | The background of the **page type being edited** (cover, profile, notes or back; the note under the title says which). One step each for a colour token, a shape or sticker (full page), or a design image (`bg.media`, `display` variant, full-bleed). *Remove background* clears it. *Use on all page types* copies the current background to the four types as **one grouped history step** (one undo). The nothing-selected panel has the colour swatches and a button to this tab. |

**Adding** puts an `asset` item on top of the edited page: 25 % of the page width, the SVG's aspect ratio, centred, selected, and the keyboard focus moves to it so the keys of T-160 act on it. An `image` item is centred with its long side at 50 % of the page width, `fit` `contain`. Refused at 40 items on a page or 150 in the book, with the message from `editor.refused.*` (not silent). A frame is an asset like the rest: "behind the photo" is the stacking buttons. Dragging from the palette onto the page is not built.

**Item controls** (property panel): a sticker, shape or frame whose SVG is `tintable` gets the colour tokens of the template and *Original colours* (removes `tint`); the others show no tint control. An image has *Fit* (whole image or fill the box), *Corner radius* (0 to 500, a slider with its value, one step per drag or key burst) and *Replace image* (the same list and upload, the box is kept). The client file name is never shown (the server does not store it). *Duplicate* (button, Ctrl or Cmd + D) copies any item 3 % of the page width right and down, on top, as one step; the delete button and the Delete key already existed.

## Uploads

At most 2 run at once (the server allows 2 per user and answers `ERROR_RATE_LIMITED` to a third), the rest wait ("Waiting"); the cap of 4 in the spec is an upper bound. The same file chosen twice while it is queued or running (a double click) is one upload. Each shows a progress bar and *Cancel*. Errors, in vi and en (`editor.upload.error.*`):

| Server answer | Message |
|---|---|
| client: not JPEG, PNG or WebP | only those types |
| client: over 10 MiB, or `ERROR_TOO_LARGE` | larger than 10 MB |
| `ERROR_UNSUPPORTED_MEDIA_TYPE` | only JPEG, PNG and WebP |
| `ERROR_INVALID_IMAGE` | could not be read, or too large in pixels: "at most 12000 px a side; JPEG up to 50 megapixels, PNG and WebP up to 25 megapixels". The numbers are `IMAGE_LIMITS` in `web/src/api/yearbooks.ts`; `imageLimits.test.ts` reads `maxSide`, `maxPixels` and `maxPixelsPNGWebP` from `internal/media/image.go` and fails when they differ (T-164) |
| `ERROR_DESIGN_IMAGE_LIMIT` (409, T-164) | **the cap**: "already has N of 30"; N is `data.limit` of the answer |
| `ERROR_QUOTA_EXCEEDED` | the 500 MiB storage limit (the 30-image cap no longer shares this code) |
| `ERROR_RATE_LIMITED`, `ERROR_BUSY` | "try again in N s" from `Retry-After` |
| `ERROR_EMAIL_NOT_VERIFIED` | verify first, with a link to `/verify-email` (new tab) |
| no connection | offline |
| `ERROR_REQUEST_TIMEOUT` and anything else | stalled / try again |

A phone photo of 4000 px is accepted: the server resizes to 3000 px (T-152).

## Deleting a design image, and why the history is cleared

The server refuses a layout that points at an image that is not the owner's, and **undo could bring such a layout back** after the image is gone. So an image that is **in use** (an item or a background on any page type; the dialog says "used on 1 page (the cover)" / "used on 2 pages (...)", i18n plural keys `inUse_one` and `inUse_other`) is deleted in this order (`palette/deleteImage.ts`, unit-tested with the real store and a fake server):

1. one history step removes every item and background that uses it;
2. `store.settle()` waits until the server has that layout; if it cannot be saved the image is **not** deleted and the dialog says so;
3. `media/delete`; if this fails the layout is already saved without the image (nothing is lost on the server), the image stays in the list and the error is shown;
4. the undo and redo stacks are cleared (`store.clearHistory()`), and the dialog said so before the click.

An image counts as in use when the current document **or any step of the undo or redo stacks** uses it (`store.usesImageInHistory`, T-164). If only the history does (the item was removed earlier), the dialog says "No page uses this image now, but your undo and redo steps still do", step 1 and the wait are skipped, and steps 3 and 4 run as above, so undo cannot bring the item back. An image that is **in no page and in no step** is deleted with a plain confirmation and the history stays. (A history that was cleared elsewhere, or a layout saved by another tab, can still point at a deleted image; the server then refuses the save and the editor shows the problem list with *Remove the items the server refused*.)

An image that a loaded layout names but the server no longer has is drawn on the page as a hatched "Missing image" box that can be selected and deleted (the print and the class book draw nothing, as before).

## Limits, cost, licence

Thumbnails are native lazy `<img>` of the `thumb` variant; a design image item is drawn with the `print` variant (the book's one renderer, 1800 px), a background with `display` (3000 px). Nothing is fetched from a third party and there is no new dependency. Every asset has a licence file under `web/src/book/assets/licences/` (docs/assets.md).

## Tests, and what is not proven

Unit: `palette/*.test.ts` (search, add defaults, upload state machine and every error mapping, the delete sequence), `document.palette.test.ts`; component: `EditorPalette.test.tsx`; browser: `tools/e2e/editor-palette.spec.mjs` (see docs/e2e.md); print: the T-154 fixture of `tools/print-check` already has an asset, an image and a background on one page.
Not proven by a machine: two tabs deleting an image (expect the conflict banner on the second tab's next save), a 4000 px phone photo, a 12 MiB file through a real network, a screen reader reading the tiles and the live region, touch use of the palette, and the rate-limit answers from the real server (mapped in unit tests only).
