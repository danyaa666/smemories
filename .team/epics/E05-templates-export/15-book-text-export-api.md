# E05_T-093 — Book as plain text with photo links (API)

**Epic:** E05-templates-export · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P1 · **Type:** feature
**Depends on:** T-092 (link token and the public-media endpoint). **Read first:** `docs/api-contract.md`, `docs/go-conventions.md`, `docs/note-fields.md`, T-056 spec (`07-book-data-endpoint.md`).

#### Description
`GET /api/book-export/get-text` builds one UTF-8 text document of the owner's book: yearbook info, owner profile, the approved notes in book order, and for every photo a public link made with the T-092 token. The server builds it (not the web) so the secret stays on the server and the 300-note cap of the print preview does not shorten an export.

#### Requirements (SHALL)
1. **Endpoint.** `GET /api/book-export/get-text?yearbook_id=&lang=` (session; namespace `book-export`). `lang` is `en` or `vi` and defaults to the book's language; anything else is `ERROR_PARAM`. Answers the v2 envelope `data: {filename, text, note_count, photo_count, approved_total, truncated}`. Ownership, malformed id and the canonical upper-case ULID rule exactly as T-056 (`ERROR_NOT_FOUND` for another user's or a missing book, `ERROR_PARAM` for a malformed id); verified email as the other owner reads. Limiter `book_export` 30 per 10 minutes per user, fail open (`ERROR_RATE_LIMITED`).
2. **Content.** Only `approved` notes, in the same order as `note/get-list?status=approved` and the print preview (`sort_order`, then creation). Photos: the profile photo, the cover photo and up to three photos per approved note, each as `<SMEM_PUBLIC_BASE_URL>/api/public-media/get-content?token=<token>` (T-092). Never a pending or hidden note, never an internal id, storage key, hash or email.
3. **No silent cut.** Use the same query shape as T-056 (one query for notes, one for photo links, no N+1, proven by a query-count test) with a hard limit of 1000 notes. Over the limit the text contains the first 1000, `truncated` is true, `approved_total` is the real count and the text itself says "1000 of N notes are included" (localised).
4. **Format** (one UTF-8 file, `\n` line ends, no BOM, NFC, control characters already excluded by the note rules; user text is never altered except for the indent below):
```
SMemories — <title>
<School label>: …   <Class label>: …   <Graduation year label>: …   <Motto label>: …   (only the filled ones, one per line)
<Exported label>: 2026-10-19   (UTC date)

== <About the owner> ==
<Full name label>: …   (every filled profile field: nickname, quote, hobbies, future plans)
<Photo label>: <link>
<Cover photo label>: <link>

== <Notes from friends> (3) ==

--- 1 / 3 ---
<field label>: <answer>            one line per answered field, in the template's field order (the note-field catalogue labels, `docs/note-fields.md`)
<Long answers on their own lines, each line indented two spaces>
<Photo label> 1: <link>
```
Labels are in the chosen language. English: School, Class, Graduation year, Motto, Exported, About the owner, Notes from friends, Full name, Nickname, Favourite quote, Hobbies, Future plans, Photo, Cover photo. Vietnamese (same wording as the memory-book template): Trường, Lớp, Năm tốt nghiệp, Châm ngôn, Xuất ngày, Giới thiệu về mình, Lời nhắn từ bạn bè, Họ và tên, Biệt danh, Câu nói yêu thích, Sở thích, Dự định tương lai, Ảnh, Ảnh bìa. Empty fields are left out; an owner with no profile text still gets the title block and the notes. Zero approved notes is valid (the section says "No notes yet" / "Chưa có lời nhắn nào").
5. **Filename.** `<title folded to ASCII, lower case, - between words, at most 40 characters>-<yyyymmdd>.txt`, fallback `yearbook-<yyyymmdd>.txt`; only `[a-z0-9-.]`.
6. **Template-aware.** The answered fields and their order come from the book's template manifest (`internal/templates`); a book without a template uses the default field set; a field the template no longer asks for but the note has is still printed after the template's fields (nothing the friend wrote is dropped).

#### Acceptance criteria
- [ ] AC1 — Golden-file tests for `en` and `vi`: Vietnamese names with combining marks (NFC), emoji, a multi-line long answer, a note with three photos, a note with no optional fields, a book with no template, zero notes, a book whose profile is empty.
- [ ] AC2 — Integration: only approved notes appear; a hidden note's photo link is not in the text; the links in the text open anonymously (T-092) and the profile/cover links too; after `revoke-photo-links` an old export's links 404 and a new export's work; another user's book is `ERROR_NOT_FOUND`; query-count test; a 1001-note book sets `truncated` and says so in the text.
- [ ] AC3 — The text never contains an internal id, storage key, hash or email (assertion over a fixture book).
- [ ] AC4 — `openapi.yaml`, `schema.d.ts`, Postman collection (runs twice), `docs/api-contract.md` route table, a short `docs/book-export.md` (format, limits, privacy note pointing at D-32 and T-092) updated; `make lint test test-integration` green.

#### Risk
`low`: an owner-only read of data the owner can already see. The sensitive part (public links) is T-092.

#### Test plan
QA probes: Vietnamese long names, emoji, 2000-character message, 40 notes, a book with 1001 approved notes, A5/A4/Letter books (no effect), a template without `relationship`, copy the file into Notepad/TextEdit/Word (encoding), open every link in a private window.
