# E11_T-096 — Web: signature pad on the public form and the signature in the inbox

**Epic:** E11-signatures · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1b · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-095. **Read first:** the PRD (L-14), the public form (T-044 and its tests), the inbox (T-040), the i18n parity check, `web/src/api/`.

#### Description
A friend signs with a finger, pen or mouse on the public form; the owner sees the signature on the note card in the inbox.

#### Requirements (SHALL)
1. **Pad component** `SignaturePad` (`web/src/components/`): one `<canvas>` using Pointer Events, `touch-action: none` on the canvas only (the page still scrolls everywhere else), device-pixel-ratio aware, logical space 1000 x 400 (from `data.signature.w/h`), full width, aspect 5:2, at least 120 px tall at 375 px. Drawing is black on a light "paper" background with a dotted baseline. Buttons **Undo** (last stroke) and **Clear**. Strokes are smoothed for display; the **stored points** are integers, thinned (minimum distance 2 units, then Ramer-Douglas-Peucker with epsilon 1.5) to fit the limits from the API (12 strokes, 600 points, **and the 3072-byte canonical JSON cap**). The byte cap usually binds first, at about 400 points with 3-digit coordinates (found on T-095), so the pad measures the serialised canonical JSON after each stroke. If it is over 3072 bytes, the pad re-thins with a larger epsilon (step 0.5, at most 4.0) before refusing. When a new stroke would still exceed any limit the pad refuses it and shows "Your signature is too detailed. Clear it and sign more simply." No timing, pressure or pointer type is read into the data.
2. **Form.** Shown only when `data.signature.enabled`, under the message, labelled "Sign here (optional)"; absent when empty (nothing is sent). A non-empty signature is sent as the multipart field `signature` with the canonical JSON. Server error `ERROR_INVALID_SIGNATURE` shows a localised message and keeps the pad. The "Sent!" screen shows the signature small, as a receipt.
3. **Inbox.** A note with a signature shows it as inline SVG at the end of the card (max height 56 px, accessible name "Signature of {name}"); the SVG `d` string is built only from the validated integers (`Number.isInteger`, within `w`/`h`), anything else renders nothing. No `dangerouslySetInnerHTML`, no `<img src=data:>`. Follows the note's status (pending, approved, hidden) like the text.
4. **Accessibility.** The pad is optional and says so; it is not a keyboard trap (it is not focusable for drawing; Undo and Clear are normal buttons); an `aria-live` region announces "Signature added", "Signature cleared"; `prefers-reduced-motion` and forced-colors modes work (canvas colours come from CSS system colours in forced-colors).
5. **Strings** in `en` and `vi` (parity check green). Vietnamese, the leader's proposal, used verbatim: "Ký tên tại đây (không bắt buộc)", "Hoàn tác", "Xóa", "Chữ ký quá chi tiết. Hãy xóa và ký đơn giản hơn.", "Đã thêm chữ ký", "Đã xóa chữ ký", "Chữ ký của {name}".
6. **E2E.** Extend `tools/e2e/journey.spec.mjs`: the friend draws with the mouse (a recorded polyline) and sends; the owner's inbox shows the signature (SVG with a path). The signature content is not printed in test output.

#### Acceptance criteria
- [ ] AC1 — Unit tests: thinning and quantisation (golden polylines), limit refusal, serialisation equals the T-095 canonical form (shared fixture with the Go validator test: the same JSON file passes both).
- [ ] AC2 — Component tests: draw, undo, clear, too-detailed refusal, optional (empty sends no field), error path, SVG builder rejects non-integers and out-of-range values.
- [ ] AC3 — Real-browser check at 375 px (touch emulation) and 1280 px (mouse), screenshots in the PR; scrolling the page by dragging outside the canvas still works; high-DPI check.
- [ ] AC4 — i18n parity, lint, `make e2e` green locally and in CI.

#### Risk
`low`: web only; validation and storage are T-095.

#### Test plan
QA probes: iOS Safari and Chrome on Android with a real finger (OWNER-TESTS style notes), a stylus, palm rejection, rotating the phone mid-stroke, very fast scribble (point budget), resizing the window, Vietnamese UI strings, a note with signature but empty message (not allowed by T-095 rules: the message stays required).
