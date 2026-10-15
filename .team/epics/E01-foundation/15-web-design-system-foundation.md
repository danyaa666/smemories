# E01_T-083 — Web: design system foundation (tokens, fonts, base components, yearbook shell)

**Epic:** E01-foundation · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature
**Depends on:** none (T-003 and T-016 are done). Blocks T-017 and T-018. Source: `docs/design/notes/` (see its README); owner decision on 2026-10-15 to adopt the Claude Design phase-1 look.

#### Description
The owner's design phase 1 (design system, public note form, notes inbox) defines the visual language of the site: warm paper, ink, tomato and teal, polaroid photos, Fraunces + Be Vietnam Pro + Patrick Hand. This task lands the shared foundation once so T-017 and T-018 only compose screens. It restyles what exists; it adds no features.

#### Scope
- In: `web/src` styles, fonts, base components, the yearbook shell, restyling of the existing screens' controls (no behaviour change).
- Out (do not do): the notes inbox and note form screens (T-017, T-018), the landing page, book preview/print, dark theme (not designed), new UI frameworks or CSS-in-JS.

#### Requirements (SHALL)
1. **Tokens.** Port `docs/design/notes/tokens.css` to `web/src/styles/tokens.css` unchanged in names and values (it is the contract with the design), imported once from `main.tsx`. Remove its Google Fonts `@import`.
2. **Fonts, self-hosted.** Add `@fontsource-variable/fraunces`, `@fontsource/be-vietnam-pro` (400, 500, 600, 700) and `@fontsource/patrick-hand`, **only the `latin`, `latin-ext` and `vietnamese` subsets**, `font-display: swap`. No request to Google Fonts or any third party at runtime (privacy: visitor IPs; and the production CSP). The three packages are OFL-licensed: add them to `THIRD_PARTY_NOTICES.md`. Reasoning recorded as a leader decision (L-12).
3. **Base components** in `web/src/components/ui/`, each with a Vitest test (roles, names, keyboard) and typed props: `Button` (primary, secondary, ghost, danger; 44 px min, 52 px primary on mobile; busy and disabled states), `TextField`/`TextArea` (label, help, error text under the field with `aria-describedby`, `role="alert"` on error, counter), `Badge` (pending, approved, hidden), `Toast` (with action button, `role="status"`), `Dialog` (confirm; focus trap, Escape, returns focus), `Banner` (offline, error), `Skeleton`, `EmptyState`, `PhotoTile` (polaroid frame, optional tape, remove button).
4. **Existing controls** (`TextField`, `SelectField`, `CodeField`, `GoogleButton`, `ResendCode`, `LanguageSwitcher`, auth and yearbook forms) use the tokens and the new base components; behaviour, routes and tests stay as they are. Focus ring is `--focus-ring` everywhere; `prefers-reduced-motion` honoured (tokens already zero the durations).
5. **Yearbook shell** `YearbookLayout`: at >= 960 px a 256 px left sidebar (brand, yearbook card, nav items My profile page, Photos, Note links, Notes from friends with a pending-count badge, Preview and print, user chip, language switcher); below 960 px a bottom tab bar (Profile, Notes, Links, Book). Nav items whose route does not exist yet are not rendered (T-017 and the print task add theirs through one small config array). The user chip shows the display handle when the user object has `username` (T-081/T-082), otherwise the email local part.
6. **Quality.** Text contrast 4.5:1 (the token file documents the ratios), touch targets >= 44 px, works at 375 px with a 16 px gutter and no horizontal scroll, keyboard and screen-reader usable, no layout shift when the fonts load (`size-adjust`/fallback metrics or `font-display: swap` accepted, say which).
7. **i18n.** Every new string is in both `en.json` and `vi.json` (parity check passes); Vietnamese copy is flagged in the PR for the owner's review.

#### Acceptance criteria
- [ ] AC1 — `web/src/styles/tokens.css` equals the design's file apart from the removed `@import` (a test or the PR diff shows it); fonts are served from our origin (network tab: no `fonts.googleapis.com` or `fonts.gstatic.com`).
- [ ] AC2 — Each base component has a passing Vitest suite; the Dialog traps and restores focus; Toast action is reachable by keyboard.
- [ ] AC3 — Existing screens still pass their tests; screenshots in the PR show Login, Register, Verify email, Account, Yearbooks and Edit at 375 and 1280 using the new look.
- [ ] AC4 — `YearbookLayout` renders the sidebar at 1280 and the tab bar at 375 (tests with matchMedia stubs); hidden items stay hidden.
- [ ] AC5 — `npm run typecheck lint test check:api`, `make lint build test` green; bundle size change reported in the PR (fonts are separate files, lazy by unicode-range); no `.team/` files in the PR.

#### Test plan
- QA should probe: Vietnamese text (long strings, diacritics) in every component at 375 px, tab order and focus ring, reduced motion, 200% zoom, offline font loading, the shell on a 600 px and a 960 px boundary.
