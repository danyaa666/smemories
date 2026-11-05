# E09_T-171 — Stretching shapes: rectangle and ellipse that fill their box (registry + web)

**Epic:** E09-customisation-editor · **Milestone:** M4 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** none. **Read first:** `docs/assets.md`, `web/src/book/assets/index.tsx` and `registry.json` (T-151), `internal/assets` (registry loader and checks, licence files), `web/src/book/layout/runtime.tsx` (how `asset` items draw), `web/src/book/editor/gestures/` (aspect-ratio rules for resize), ADR 0005 Q1 (why: lines/rectangles are 0 to 17% convertible because our shapes keep their aspect ratio).

#### Why
Every asset today keeps its aspect ratio, so there is no plain rectangle or ellipse that can be any width and height. That blocks banners, cards, dividers and frames, and it is the main reason imported or authored templates lose decoration (ADR 0005: 73% of objects convert, 85% with two stretching shapes).

#### Requirements (SHALL)
1. Two new registry assets, **`shape.rect`** (a plain rectangle, no corner radius) and **`shape.ellipse`**, authored by us (CC0, licence file per asset like the rest of the pack, T-151 rules), tintable with the manifest colour tokens, flagged `stretch: true` in `registry.json` (documented in `docs/assets.md`: what the flag means).
2. `AssetSvg` draws a `stretch` asset edge to edge in its box (`preserveAspectRatio="none"`; strokes keep their width, `vector-effect: non-scaling-stroke` where an outline exists); non-stretch assets are unchanged.
3. The editor lets a `stretch` asset be resized freely in width and height (no locked aspect on the edge handles; corner handles keep the existing modifier behaviour); the palette adds them to *Shapes*, added at a sensible default (e.g. 40% page width, 25% height); keyboard resize works as for other items. Other assets keep today's behaviour.
4. Server validation needs only the registry change; the validator's box limits apply as for any asset item (verify, add a test with an extreme aspect ratio such as w 900, h 10).
5. Print parity: add the two shapes to the print-check fixture (stretched, rotated, tinted) on A5/A4/Letter; edge anti-aliasing only.
6. Tests: registry/asset unit tests (flag present, licence file present, registry check green), runtime test (drawn edge to edge), gesture test (free resize only for `stretch`), `go test ./internal/assets/... ./internal/booklayout/...`, a Playwright case added to the palette spec (add a rect, resize it to a thin banner, reload, same).
7. Docs: `docs/assets.md` (the flag, the two assets, licence), palette section in the editor docs.
8. No new dependency; nothing under `.team/`; no migration.

#### Acceptance criteria
- [ ] AC1 — A student adds a rectangle and an ellipse, resizes each to any width and height, tints them, reloads and sees the same; they print identically (print-check green).
- [ ] AC2 — Other assets keep their aspect ratio and look; registry and licence checks green.
- [ ] AC3 — Lint, prettier, tsc, vitest, `lint:i18n`, `check:api`, build, Go tests of the touched packages, e2e (incl. the new case) and `make print-check` green; QA reports counts and head SHA.

#### Test plan
QA runs everything on the PR head, tries extreme aspect ratios, rotation of a thin rectangle, tint with each theme, and prints on all three papers.
