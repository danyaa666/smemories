# Book as plain text (T-093)

`GET /api/book-export/get-text?yearbook_id=&lang=` (package `internal/bookexport`, session, owner only, verified email) answers the v2 envelope with
`data: {filename, text, note_count, photo_count, approved_total, truncated}`. The server builds the text so the link secret stays there and the 300-note
cap of the print preview (`book-data/get`) does not shorten an export. The web screen (T-094) saves `text` as `filename`.

## Format

One UTF-8 file, `\n` line ends, no BOM, NFC. User text is copied as it is; the only thing added to it is a two-space indent on the lines of a
multi-line value (blank lines inside stay empty). Empty fields are left out.

```
SMemories — <title>
School: …            (only the filled ones, one per line: School, Class, Graduation year, Motto)
Exported: 2026-10-19 (UTC date)

== About the owner ==             (left out when there is no profile text and no photo)
Full name / Nickname / Favourite quote / Hobbies / Future plans
Photo: <link>
Cover photo: <link>

== Notes from friends (3) ==
1000 of 1500 notes are included   (only when the book was cut)

--- 1 / 3 ---
Your name: Linh                   short answers on the label line
Your message:                     long answers (and any answer with a line break) on their own lines
  line one
  line two
Photo 1: <link>                   at most 3 per note
```

`lang` is `en` or `vi` (anything else is `ERROR_PARAM`) and defaults to the book's language. The fixed labels are in `internal/bookexport/text.go`;
the labels of the answers are the catalogue labels of [note-fields.md](note-fields.md). A book with no approved note says "No notes yet" /
"Chưa có lời nhắn nào". Golden files for both languages are in `internal/bookexport/testdata` (`go test ./internal/bookexport -update` rewrites them).

Answers come in the order of the book's template (`internal/templates`); a book without a template uses `notefields.Default()`. An answered field the
template does not ask for (the template was changed later) is still printed, after the template's fields, so nothing a friend wrote is dropped.

`filename` is the title folded to lower-case ASCII words joined by `-` (at most 40 characters), `-`, `yyyymmdd` (UTC) and `.txt`; `yearbook-<yyyymmdd>.txt`
when nothing of the title is left. Only `[a-z0-9-.]`.

## Limits

- Only `approved` notes, in the order of `note/get-list?status=approved` and the print preview (`sort_order`, then creation).
- At most **1000 notes**. Over that, the text holds the first 1000, `truncated` is true, `approved_total` is the real count and the text says
  "1000 of N notes are included". Never a silent cut.
- One query for the notes and one for their photo links (plus the book and its profile, plus one `COUNT` at the cap): no N+1, proven by a query-count test.
- 30 calls per 10 minutes per user (limiter `book_export`, [redis.md](redis.md), fails open): `ERROR_RATE_LIMITED` with `Retry-After`.

- Signatures are not exported: the plain-text export lists no signature (they are drawings; they appear only in the printed book).

## Privacy

Every photo is a **permanent public link** made with the T-092 signer: `<SMEM_PUBLIC_BASE_URL>/api/public-media/get-content?token=...`. Anyone holding
a link can open that photo without signing in until the owner calls `yearbook/revoke-photo-links` (which raises the book's `link_epoch` and ends every earlier
link), or until the note is hidden or deleted. This is the owner's choice in decision D-32 (README section 4); the controls are in
[media.md](media.md) ("Public photo links"). The text itself never holds an internal id, storage key, hash or email address (asserted by a test), the
response is `Cache-Control: no-store`, and neither the text nor a link is ever logged. Each call makes the links again at the current epoch.

## Class book (T-027)

`GET /api/class-book/get?class_id=` (package `internal/class`, session, class admin only) is the data of the class book for the browser renderer, the class counterpart of
`book-data/get`: `{class, front_pages, members[], back_pages}`; each member is `{member_id, name, linked, yearbook{template_id, page_size, language}, profile, notes[],
note_count, approved_total, truncated}`. Permissions and privacy: [class-permissions.md](class-permissions.md). The class pages (T-127) are in the two arrays, each `{id, kind, title, body, placement, created_at, updated_at}`, `body` plain text to be shown as text; an empty side is `[]`.

- **Budget.** The book keeps the page budget of about 120 pages: at most **60 members and 40 approved notes per member** (`notes.MaxClassBookNotes`). A student with more approved notes is cut at 40 in
  their own order, with `truncated: true` and the real `approved_total` (never silent).
- **Cost.** One query for the class, one for the members, then per linked member the book, its profile (and the photo ids) and the notes with their photo links: a handful of queries per member,
  none per note (`TestClassBookQueryCountIsPerMember`). The 60 x 5 queries are indexed (`TestClassBookQueriesUseIndexes`).
- **Read limit (T-133).** 30 reads per user and class per hour (`bookReadsPerWindow` in `internal/class/service.go`, the one constant to tune), counted after the admin check, so only an admin's successful
  authorization takes from the budget. Over it: `429 ERROR_RATE_LIMITED` with `Retry-After`. The limiter is the closed kind (like the class write limiter): when Redis does not answer, `503 ERROR_UNAVAILABLE`
  (`TestClassBookReadLimiter`).
- **Size.** Measured by `TestClassBookFullClass` (60 members x 40 notes of about 230 characters, two photo ids each): about **1.1 MiB**; the class pages add at most 30 x (4000-character body + 120-character title),
  at worst about 0.8 MiB when every character needs a JSON escape. A realistic class is never near the cap below.
- **Hard cap: 8 MiB (T-133).** The theoretical worst case is far larger: one note may hold about 4.7 thousand characters of answers (up to 4 bytes each) and a signature of up to 3 KiB, so 60 x 40 maximal notes are about
  50 MiB, which the server would build twice (marshal, then copy) for every request. The body therefore **never exceeds `class.MaxBookBytes` = 8 MiB (8388608 bytes), envelope included.** The members are read and encoded one by one in book
  order and their encoded size is added up. The first member whose notes would pass the cap loses **all** of them (a member is complete or has none), and so does every later member; they keep their profile, `approved_total`,
  `truncated: true`, `note_count: 0`, `notes: []`, and their ids are listed in the top-level `truncated` array (a member without notes is never listed). Members before the cut are complete. A class that fits has `truncated: []`.
  The cap leaves room for the worst case of what follows: the class, the pages, the list of cut ids and `strippedMemberMax` (12 KiB, the profile at its longest with every character escaped) for each member still to come,
  so the cut itself cannot break the cap (`TestClassBookStaysUnderTheCap`, `TestStrippedMemberFitsItsReserve`, `TestClassBookIsCutAtTheCap`).
  The web print route (T-130) ignores the field; it still names the members whose notes are missing from its own `approved_total` and `note_count`.
- **No second copy.** `ClassBook.Members` holds the encoded members (at most the cap, once); the handler writes the envelope and the pieces straight to the response (`httpx.OKStream`, `ClassBook.Encode`), with no `Marshal` of the whole body and no
  copy of it. Peak memory for the maximal class is measured at about 1.2 x the response and asserted below 3 x (`TestMaximalClassPeakAllocation`, run under `-race`). All the reading that can fail is done before the first byte, so an error is still a clean error envelope.
- **Photos.** The images are fetched by the admin with `media/get-content?variant=print`, which accepts exactly the profile photo and the approved notes' photos of the linked books (one SQL statement, see class-permissions.md).
- The data is not cached and not exported as text in v1; the admin prints the book in the browser.

### Printing the class book (T-130)

`/classes/:id/book` (class admin; a student sees the 403 message) prints the class book in the browser, like `/yearbooks/:id/book` prints a personal one. Order: cover (class name, school, year),
the front class pages, then for each member in book order the **existing** profile and notes pages of that student's own template (`ProfilePages` in `web/src/book/BookPages.tsx`, shared with
the personal book, whose route and templates are unchanged), the back class pages, the back cover. A member with no page prints nothing. The paper size is one for the whole book: the size most students
chose, or the one picked on the screen (`?size=A5|A4|Letter`); the cover and class pages are in the class's language, a student's pages in the student's own. A class page is plain text (React text nodes, newlines
kept) shrunk to one page; the text of a page that still does not fit is cut and listed. The screen shows the page count (a warning above 120) and a summary naming the students with no page, the students whose
approved notes were left out by the 40-note limit, whose notes or profile are cut at the template's smallest size, and the class pages that are cut. Ready (`html[data-book-ready]`, Print enabled) waits for the fonts and
for **every** photo of every student's pages, through the shared queue of at most 16 requests in flight. `tools/print-check` prints a 30-student fixture (three templates, 77 pages) on A5 and A4 (about 10 s).
