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
note_count, approved_total, truncated}`. Permissions and privacy: [class-permissions.md](class-permissions.md). The class pages (T-128) arrive in the two arrays; until then they are empty.

- **Budget.** The book keeps the page budget of about 120 pages: at most **60 members and 40 approved notes per member** (`notes.MaxClassBookNotes`). A student with more approved notes is cut at 40 in
  their own order, with `truncated: true` and the real `approved_total` (never silent).
- **Cost.** One query for the class, one for the members, then per linked member the book, its profile (and the photo ids) and the notes with their photo links: a handful of queries per member,
  none per note (`TestClassBookQueryCountIsPerMember`). No limiter: it is an admin read, and the 60 x 5 queries are indexed (`TestClassBookQueriesUseIndexes`).
- **Size.** Measured by `TestClassBookFullClass` (60 members x 40 notes of about 230 characters, two photo ids each): see the result in the task's board comment; the budget is 4 MiB.
  The theoretical worst case is far larger: one note may hold about 4.7 thousand characters of answers (up to 3 bytes each in Vietnamese) and a signature of up to 3 KiB, so 60 x 40 maximal notes would be about 40 MiB. Nobody writes that
  (the cap of 500 characters on five of the nine fields and 2000 on the message is a ceiling, a note is typically a few hundred bytes), and a book of 2400 maximal notes is not 120 pages either; the 4 MiB target holds for
  realistic notes, not as a hard limit. If it ever matters, the lever is the per-member cap (`notes.MaxClassBookNotes`), not a response-size check.
- **Photos.** The images are fetched by the admin with `media/get-content?variant=print`, which accepts exactly the profile photo and the approved notes' photos of the linked books (one SQL statement, see class-permissions.md).
- The data is not cached and not exported as text in v1; the admin prints the book in the browser.
