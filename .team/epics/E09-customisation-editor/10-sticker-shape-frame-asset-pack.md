# E09_T-151 — Asset pack and registry for stickers, shapes and frames

**Epic:** E09-customisation-editor (stage 2) · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** none. **Read first:** E09 PRD "Stage 2 plan", `docs/templates.md` (fonts and images), `web/src/book/fonts/README.md` (how licences are recorded there), owner decision D-43(d).

#### Why
The editor lets a student add stickers, shapes and frames (D-43). They come from free-licence packs curated by us, one licence file per asset, and the server must know which ids exist so it can reject anything else. This task builds the pack and the registry; the editor and the layout model come later.

#### Requirements (SHALL)
1. **Registry.** One JSON file, `web/src/book/assets/registry.json`, lists every asset: `id` (stable, lowercase, `kind.name`), `kind` (`sticker`, `shape` or `frame`), `file`, `label_en`, `label_vi`, `tags`, `tintable` (true when the SVG uses `currentColor` for one tint colour), `width` and `height` (the viewBox), `licence` (`CC0`, `OFL` or `authored`), `source_url`, `author` and `date_checked`. A copy the Go side can embed is generated into `internal/assets/registry.json` by a script, and a Go test and a web test pin both to the same ids (same pattern as the template manifests).
2. **Pack, part 1 (authored, no licensing risk).** At least 24 simple assets drawn by the team in this task: geometric shapes (circle, rounded square, star, heart, burst, arrow, speech bubble, banner ribbon), tape strips, doodle lines and underlines, 6 frames (photo corners, polaroid, rounded border, scalloped, double line, film strip). Licence `authored` (we place them under CC0).
3. **Pack, part 2 (curated).** At least 24 more stickers (school and friendship themes: books, pencils, stars, flowers, hearts, graduation cap, notes, clouds, sun) from sources that publish them under **CC0 or the SIL OFL only** (for example Openclipart and Kenney publish under CC0). Verify the licence on the source page on the day, record `source_url`, `author` and `date_checked`, keep the original licence text in `web/src/book/assets/licences/<id>.txt` (or one file per pack when the pack has a single notice). If a source cannot be reached or the licence is unclear, draw a replacement yourself; never include an asset whose licence you could not verify. Not allowed: CC BY, CC BY-SA, NC, ND, or "free for personal use".
4. **SVG safety.** Every file is a plain SVG: no `<script>`, `<foreignObject>`, event attributes, external references (`href` to a URL, `@import`, `url(http...)`), embedded raster images or fonts; at most 20 KB each; a `viewBox`; shapes and paths only. `scripts/check-assets.sh` enforces this and the registry rules (every file listed, every entry has a licence line, every licence file exists, ids unique, labels in both languages) and runs in the `web` job of CI.
5. **Use.** A small module `web/src/book/assets/index.ts` exports `assets`, `getAsset(id)` and a `<AssetSvg id tint size />` component that inlines the SVG as React elements (no `dangerouslySetInnerHTML` of user data; the SVGs are ours, imported at build time). Loaded lazily by kind so the book's print bundle does not grow when no asset is used. A test renders every asset and checks it has a non-empty bounding box.
6. **Docs.** `docs/assets.md`: how the pack is organised, how to add an asset (licence rules, steps, the check script), and how the registry feeds the server validation. List the sources used.
7. No new runtime dependency; no change to the book output; nothing under `.team/`.

#### Acceptance criteria
- [ ] AC1 — The registry has at least 48 assets (24 authored, 24 curated) with both labels, licence data and licence files; `scripts/check-assets.sh` passes and fails on a deliberately added `<script>` or a missing licence line.
- [ ] AC2 — The Go and web registries list the same ids (tests on both sides).
- [ ] AC3 — A contact sheet page (a test route or a print-check fixture, not shipped in the nav) shows every asset tinted and untinted; the PR includes a screenshot of it.
- [ ] AC4 — The print bundle size is unchanged when no asset is used (report the numbers).
- [ ] AC5 — Lint, typecheck, vitest, i18n parity, `go test ./...`, build, print-check unchanged.

#### Test plan
QA opens the contact sheet, spot-checks five licence entries against their source pages, runs `scripts/check-assets.sh` with a planted bad file, and runs the web and Go checks.
