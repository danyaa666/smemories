# E05_T-094 — Web: "Export as text" on the print screen, with the public-link warning and "disable links"

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature
**Depends on:** T-092, T-093. **Read first:** `web/src/` print screen (T-061), `docs/api-contract.md`, the i18n parity check, the existing dialog/toast patterns.

#### Description
On the "Preview and print" screen, next to "Print or save as PDF", the owner can download the book as a text file or copy the text, after being told that the photo links in it are public, and can switch every photo link of the book off.

#### Requirements (SHALL)
1. **Section "Export as text"** on `/yearbooks/:id/book`, below the print guide: a short line ("A plain text file with your profile, the approved notes and a link for every photo."), the buttons **Download text file** and **Copy text**, and a text button **Disable photo links**. Language of the file follows the book's language (`lang` omitted); no extra picker.
2. **Public-link warning.** The first Download or Copy in a browser session opens a dialog (focus trapped, Escape = cancel, works with keyboard and at 375 px): "The photo links in this file are public. Anyone who has a link can open that photo without signing in. You can switch all photo links of this book off at any time." Buttons **Continue** and **Cancel**. After Continue it is not shown again until the page is reloaded. Vietnamese: "Các liên kết ảnh trong tệp này là công khai. Bất kỳ ai có liên kết đều xem được ảnh đó mà không cần đăng nhập. Bạn có thể tắt tất cả liên kết ảnh của sổ lưu bút này bất cứ lúc nào." Buttons "Tiếp tục" / "Hủy".
3. **Download** calls `GET /api/book-export/get-text`, builds a `Blob` (`text/plain;charset=utf-8`) and saves it under `data.filename` (anchor with `download`, object URL revoked after). **Copy** uses `navigator.clipboard.writeText` with a fallback to a selected, read-only `<textarea>` when the clipboard API is unavailable or refused; a toast says "Copied" / "Đã sao chép". If `truncated` is true a visible localised notice says "{note_count} of {approved_total} notes are included".
4. **Disable photo links.** Opens a confirm dialog ("Every photo link you have already shared stops working. A new export creates new links. Continue?" / Vietnamese "Mọi liên kết ảnh đã chia sẻ sẽ ngừng hoạt động. Lần xuất mới sẽ tạo liên kết mới. Tiếp tục?") and calls `POST /api/yearbook/revoke-photo-links`; success toast "Photo links disabled" / "Đã tắt liên kết ảnh". The next export prompts the warning again.
5. **States.** Buttons disabled and labelled while a request runs (no double submit); `ERROR_RATE_LIMITED` shows the localised retry message; any other error shows the generic localised error and leaves the page usable; the section never blocks printing.
6. **Strings** in `en` and `vi` (key parity check stays green); the Vietnamese texts above are the leader's proposal: dev uses them verbatim, QA checks diacritics.
7. **E2E.** Extend `tools/e2e/journey.spec.mjs` after the print step: download the text (Playwright download event), assert it contains the title, the three note names and one photo link; fetch that link from a **fresh context with no cookies** (200, `image/*`); click Disable photo links and assert the same link is now 404 and the page shows the toast. The token never appears in test output or traces (scrub it like the existing collection token).

#### Acceptance criteria
- [ ] AC1 — Component tests: warning shown once per session, Cancel does nothing, Continue triggers the request; download builds the Blob and filename; copy success and clipboard-refused fallback; truncated notice; disable-links confirm and error paths; no double submit.
- [ ] AC2 — Real-browser check at 375 px and 1280 px (screenshots in the PR): the section, both dialogs, the toast; keyboard-only run through both dialogs.
- [ ] AC3 — i18n parity and lint green; the e2e journey passes locally (`make e2e`) and in CI, including the anonymous link fetch and the revoke step.
- [ ] AC4 — `docs/e2e.md` mentions the new steps.

#### Risk
`low`: web only; the privacy controls live in T-092.

#### Test plan
QA probes: Safari (download attribute, clipboard), Firefox, Chrome on Android, a 200-note book, Vietnamese book and English UI, a book with no photos, double-click on Download, offline, session expired mid-request (401 handling as elsewhere).
