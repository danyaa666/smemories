# Claude Design export: site UI, phase 1

Source: the owner's Claude Design project (https://claude.ai/design/p/ffda8bd0-0727-4c4b-b840-608538099734), exported 2026-10-15.
Files here are **reference only** (not shipped, not imported by the app). Open the `.dc.html` files in a browser (they load `support.js` and Google Fonts).

| File | What it is |
|---|---|
| `tokens.css` | The design tokens (colours with contrast ratios, type scale, spacing, radii, shadows, motion). Ported into `web/src/styles/tokens.css` by T-083. |
| `Design System.dc.html` | Tokens, type, buttons, fields, badges, toasts, empty states on one canvas. |
| `Public Note Form.dc.html` | Screens N1 to N11: the form (375 and 1280), filled, errors, slow connection, sent, Vietnamese, dead ends (expired, full, revoked, photo failed). Used by T-018. |
| `Notes Inbox.dc.html` | Screens I1 to I6: inbox desktop and mobile (clickable), reorder mode, first run, loading, offline. Used by T-017. |

The full export (with the eight book-template references `uploads/`, `_ref/`, 34 MB) stays on the owner's machine in `design/notes/` and is not committed.
Decisions made in the design: fonts Fraunces (headings), Be Vietnam Pro (body) and Patrick Hand (short accents) because Caveat, Karla and Jost lack Vietnamese glyphs;
photos are striped placeholders; the dark theme is not designed. **Where the design and a task spec differ, the spec wins**; each spec lists the known differences.
