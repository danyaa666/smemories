# E11 — Friends' handwritten signatures

**Status:** planned (next phase, after the go-live phase)   **Milestone:** M1b   **Owner decisions:** owner request 2026-10-19 ("add gather sign for next phase", confirmed: handwritten signatures), D-01, D-24; leader decision L-14 (README §4)

## Problem and users
A paper yearbook is signed. Our notes carry words and photos but not the friend's own hand, which is what makes a "lưu bút" feel personal. A friend opening the link on a phone should be able to sign with a finger, and the signature should appear in the printed book next to their message.

## Goal and non-goals
- Goal: the public note form offers an optional "Sign here" pad; the signature is saved with the note, moderated with it (pending, approved, hidden, deleted together), shown to the owner in the inbox and printed by the two system templates.
- Non-goals: signature verification or legal signatures, the owner signing their own page, typed "signature fonts", stickers or free drawing on pages (that is the E09 editor), colours or pen sizes chosen by the friend, handwriting recognition, animation or replay, exporting signatures as image files.

## User stories
- As a friend I draw my signature on my phone, undo a stroke, clear it, or skip it entirely, and send my note.
- As an owner I see the signature in the inbox card of the note and I approve or hide it with the note.
- As an owner my printed book shows each approved signature under the friend's message, sharp at print size.

## Design decisions (L-14)
- **Vector strokes, not images.** A signature is a short list of integer points (`{"v":1,"w":1000,"h":400,"s":[[x0,y0,x1,y1,...],...]}`), at most 12 strokes, 600 points and 3072 bytes. It is re-serialised by the server (the client string is never stored), drawn by our own code as an SVG path built from validated numbers, **never** as user-supplied markup or an uploaded file. Consequences: no image pipeline cost, no EXIF, no XSS surface, crisp at any print size, tiny storage.
- **No biometrics.** Only positions are kept: no timing, pressure, speed, pointer type or device data (a signature's dynamics can identify a person).
- **Template-driven.** A template manifest declares `"signature": true` like it declares note fields (D-21); only then does the public form show the pad and the API accept a signature.
- **Same moderation.** A signature is part of the note row: pending until approved, hidden and deleted with the note, included in data export and account deletion (E06 T-025) automatically.
- **Optional for the friend**, so keyboard-only users and people who do not want to sign lose nothing.

## Tasks (all BACKLOG until the go-live phase is done; specs in this folder)
| Task | Spec | Depends on |
|---|---|---|
| T-095 Signature storage, validation and API | `01-signature-api.md` | — |
| T-096 Web: signature pad on the public form and in the inbox | `02-signature-pad-web.md` | T-095 |
| T-097 Print: signatures in memory-book and navy-classic | `03-signature-print.md` | T-095 |

## Risks
- Public input: bounded, validated, rate-limited like notes (T-095 risk high).
- Response size: book-data grows by up to 3 KB per note (T-095 measures and re-states the T-056 bound).
- Offensive drawings: covered by moderation (pending by default).
- Print clutter: the signature must never overlap text (T-097).
