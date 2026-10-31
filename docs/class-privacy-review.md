# Class yearbook: privacy review (E07, T-131, decision D-36)

The privacy review of the class feature, written before the first production use. It answers four questions: what personal data a class adds, who sees it,
how long it lives and what happens when someone leaves, and what an attacker or a careless admin can do with it. It ends with the owner's sign-off, which
is **not given yet**. The contract it reviews is [class-permissions.md](class-permissions.md); the texts users accept are in [legal/](legal/README.md)
(version `2026-10-29`, still marked DRAFT).

## 1. What personal data a class adds

| Data | Where | Who it is about |
|---|---|---|
| The class: name, school, graduation year, language, who created it | `class_tab` | the admin (creator id), the students (indirectly: the name of their class) |
| Membership: that a person is in the class, with which role, since when, in which book position | `class_member_tab` | every member |
| The student's chosen page: a pointer to one of the member's own yearbooks | `class_member_tab.yearbook_id` | the student |
| Invite metadata: SHA-256 of the token (never the token), creator id, expiry, revoked time | `class_invite_tab` | the admin who made it |
| Class pages (intro, teachers, events, superlatives, custom): plain text written by an admin | `class_page_tab` | whoever the admin names in the text |
| **Not stored, only shown (assembled on read):** the student's profile, approved notes (with the authors' names, relationships, messages, signatures) and the photos of the profile and of approved notes | the member's own yearbook (`class-book/get`, `media/get-content`) | the student and **the friends who wrote notes** |
| **Design choices and fixed-text edits** (T-146): the student's resolved design (theme, font set, section order and hidden sections, replaced titles and texts, picture choices), shown only to the class admin in `class-book/get` | the member's own `book_design_tab` row, read as the book's owner | the student: the only free text in it is what they typed into the template's text slots (titles), which they chose to put in their book; no other personal field is added |
| **Design images** (T-152, D-43): pictures the student uploads as a background or decoration for their own book (`media_tab.purpose = 'design'`, at most 30 per book). They go through the photo pipeline (metadata stripped), are the student's own choice and are not photos of friends, but the class admin can open them (`media/get-content`, read only, `no-store`) for every member whose book is linked to the class, **including images no layout uses yet**. | the student, and anyone who appears in a picture the student chose to upload |

No email address, user id, birthday, cover photo, note-link token, pending or hidden note is ever part of what the class layer shows an admin. The roster shows
a display name, role, join time and three facts about the page (`page_linked`, `profile_complete`, `approved_notes`).

Two points the owner should know about:
1. **A friend's note reaches a third party.** A friend wrote a note for a classmate. Once the classmate approves it and links their book to a class, the class admin
   (a teacher or monitor the friend may not know) can read it and print it. The legal texts now say so (privacy section 4, terms sections 6 and 7), but the public note form
   does not warn the friend at the moment of writing. Recommended follow-up: one line on the form when the book is linked to a class (needs a product decision).
2. **Age.** The terms say accounts are for people 18 and older (D-35, unchanged). A school class is often under 18. The feature does not check age. Whether it is
   offered to minors, and the consent that would need, is an owner decision and is listed in the sign-off block.

## 2. Who sees what (the matrix)

Enforced in SQL on `class_member_tab` and tested endpoint by endpoint (section 6). "Never" means no endpoint returns it to that role.

| Data | Class admin | Student (member) | Member of another class / signed-in non-member | Anonymous |
|---|---|---|---|---|
| Class name, school, year | yes | yes | no (404, same as "no such class") | no (401) |
| Roster: display names, roles, join time, page flags | yes | **no** (403; own row only in `class/get`) | no | no |
| Another student's profile, approved notes, photos | yes, read only, through `class-book/get` and `media/get-content` | **never** | never | never |
| A student's design images (backgrounds and decorations, T-152) | yes, read only, through `media/get-content`, for members whose page is linked to the class (`no-store`) | own only | never (404) | never |
| Their own page, notes and photos | n/a | yes (normal yearbook endpoints, owner only) | n/a | no |
| A student's pending or hidden notes, note-link tokens, unapproved photos, cover, email, user id | **never** | own only | never | never |
| Edit or moderate a student's yearbook | **never** (404 on every yearbook, note, media, book-data and book-export route) | own only | never | never |
| Class pages and the book order | read and write | **no** (403) | no | no |
| Invite links: make, list, revoke | yes (the token only once, at creation) | **no** (403) | no | no |
| Invite preview (`class-invite/lookup`): class name and school | yes | yes | yes, with a live token | yes, with a live token |

Students never see each other's notes, pages or the roster. Class admins can always see the above for the **whole** class, which is the purpose of the feature and is
disclosed in both privacy policies and terms.

## 3. Retention: what happens when someone goes

| Event | Class data | Student's yearbook | Admin's access to the student's page |
|---|---|---|---|
| Student leaves (`class-member/leave`) | member row deleted | untouched, still the student's | ends at once (next request; no server cache) |
| Admin removes a student (`class-member/remove`) | member row deleted | untouched | ends at once |
| Student unlinks the page (`class-member/set-book` with `null`) | link cleared, membership kept | untouched | ends at once |
| Student deletes the yearbook | link cleared by the yearbook purger, membership kept | deleted with its notes, photos and design images (existing rules) | ends at once |
| Student deletes a design image (`media/delete`) | none | the image and its three stored versions are deleted | ends at once |
| Student hides or deletes a note | none | note hidden or deleted | that note disappears from the next `class-book/get` |
| Admin deletes the class (`class/delete`) | members, invites and class pages deleted in one transaction | **never touched** | ends; nobody keeps access |
| Account deletion | no endpoint exists yet (see the legal texts' bracketed item). T-025 must delete the design images with the books (they are `media_tab` rows of the book, so the yearbook delete path covers them) and list them in the export (`GET /api/media/get-list?purpose=design` per book) | n/a | n/a |

What stays after the access ends: pages an admin already **printed or saved as PDF** from their browser, and images the browser cached (`private, max-age=3600`).
That is outside the server's control and the policy says so. Backups follow the retention of the other data (`[N]` days, to be filled by the owner).
Logs: the API's access log records the route only, never a query string or body, and the invite token is read from a POST body and never logged
(test `TestInviteTokenIsNeverLogged`); the Playwright trace scrubber and the edge log rule redact `/join/<token>` as they do `/c/<token>`.

## 4. The admin leaves, is demoted or removed

- A class always has an admin. The last admin can neither leave, be removed nor be demoted (403 `ERROR_LAST_ADMIN`), also under concurrency. To leave, an admin first
  promotes someone else, or deletes the class.
- When one of several admins leaves, is demoted or is removed, they lose every admin right at the next request: roster, invites, class pages, class book **and the media read
  path** (it joins `class_member_tab` on every call; `TestAdminMediaReadPath`, `TestDemotedAdminCannotWritePages`).
- Their **active invite links are revoked** in the same transaction as the change (owner decision D-40 c, T-140): `revoked_at` is set on every unrevoked, unexpired invite
  of that class made by that person, under the class row lock, before the invite rows are locked (the lock order of accept). People who already joined with those links
  stay members; the other admins' links are untouched; `class-invite/get-list` shows the revoked ones as revoked. A person who is only a student has no links (no-op). Tests:
  `TestDepartedAdminsInvitesAreRevoked` (leave, remove, demote), `TestInviteAcceptRacingTheCreatorLeaving` (an accept racing the exit is either before it or refused).
- Class pages the departed admin wrote stay with the class (they are the class's, not the person's).

## 5. Abuse cases

| Case | What happens | Why it is bounded |
|---|---|---|
| **Invite link leaks** (posted in a public chat) | a stranger can join as a student | a student sees only the class name and their own row; they read nothing of anyone else (`class-book/get` 403, roster 403, other yearbooks 404). The admin sees every new member in the roster and removes them or revokes the link; links expire (default 14 days) and a class holds at most 5 live links and 60 members. The token is 256 bits, stored only as a hash, shown once, never logged. A stranger who joins and links a page can only add their own content to the printed book, which the admin controls by removing them |
| **Guessing links** | not feasible | 256-bit tokens; unknown, expired and revoked tokens give one identical 404; per-IP miss limiter (20 per 10 minutes) |
| **A student wants to read a classmate's notes** | refused | every yearbook, note, media and book-data route checks `owner_id = caller`; `class-book/get` is admin only. Tests: `TestAdminStillHasNoRightsOverAStudentsBook` (admin route table), the e2e journey `tools/e2e/class.spec.mjs` (as student A against B's book and the class book) |
| **An admin account is taken over** | the attacker reads the approved content of the whole class, as the admin could | content is approved-only and read only; students can withdraw at once (leave or unlink); the admin needs a verified email and a session; no password or token is part of the content |
| **A malicious admin** | sees what students approved and linked, prints it | disclosed in the policy and terms; no write access to anyone's yearbook; the student can withdraw at any time |
| **Admin removed in anger** | the removed member keeps their yearbook and can join again only with a live link | removal never deletes the yearbook; their class data is the member row only |
| **Someone attaches an unrelated yearbook** | not possible | `set-book` accepts only the caller's own yearbook (404 otherwise); one yearbook belongs to at most one class (409) |
| **A student crafts a class id or member id of another class** | nothing changes | every id is looked up inside the class named in the request (`TestCrossClassIDsChangeNothing`, `TestPageIDsOfAnotherClassChangeNothing`) |
| **Script in a class page or a note** | shown as text, not run | the server stores plain text; the web renders it as React text (T-130 checks this) |
| **Flooding** | refused | writes limited to 120 an hour per user and fail closed; caps: 10 classes per admin, 60 members, 5 live links, 30 class pages, 40 notes per student in the book; class-book reads are limited to 30 an hour per user and cut at 8 MiB (`TestClassBookReadLimiter`, `TestClassBookIsCutAtTheCap`) |

## 6. Endpoints that expose class data, and the test that proves each role check

Every operation under `class*` in `api/openapi.yaml` has a row. `scripts/check-class-matrix.sh` (CI, job `go`) fails when an operation has no row, or when a row
names a test file or a test function that does not exist. Test files are in `internal/class/`. The matrix tests run in the integration suite
(`make test-integration`); `class-book/get` and the media read path are also driven by the e2e journey.

| Endpoint | Exposes | Test file | Test |
|---|---|---|---|
| `class/create` | creates a class, caller becomes admin | `matrix_integration_test.go` | `TestMatrixCreateAndList` |
| `class/get-list` | the caller's own classes | `matrix_integration_test.go` | `TestMatrixCreateAndList` |
| `class/get` | class and the caller's own role and page | `matrix_integration_test.go` | `TestPermissionMatrix` |
| `class/update` | changes class details (admin) | `matrix_integration_test.go` | `TestPermissionMatrix` |
| `class/delete` | deletes the class rows (admin) | `matrix_integration_test.go` | `TestPermissionMatrix` |
| `class-member/get-list` | the roster (admin) | `matrix_integration_test.go` | `TestPermissionMatrix`, `TestStudentNeverSeesTheRoster` |
| `class-member/update-role` | promotes or demotes (admin); a demoted admin's active links are revoked | `matrix_integration_test.go` | `TestPermissionMatrix`, `TestDepartedAdminsInvitesAreRevoked` |
| `class-member/remove` | removes a member (admin); a removed admin's active links are revoked | `matrix_integration_test.go` | `TestPermissionMatrix`, `TestDepartedAdminsInvitesAreRevoked` |
| `class-member/leave` | the caller leaves; a leaving admin's active links are revoked | `matrix_integration_test.go` | `TestPermissionMatrix`, `TestDepartedAdminsInvitesAreRevoked` |
| `class-member/set-book` | the caller's own page link | `page_integration_test.go` | `TestSetBookMatrix` |
| `class-book/get` | profiles, approved notes, each student's design, class pages (admin only; `TestClassBookMatrix` also proves a student, a classmate, a stranger and another class's admin never receive the design) | `book_integration_test.go` | `TestClassBookMatrix` |
| `class-book/update-order` | order of members and pages (admin) | `pages_integration_test.go` | `TestPageMatrix` |
| `class-page/create` | class page (admin) | `pages_integration_test.go` | `TestPageMatrix` |
| `class-page/update` | class page (admin) | `pages_integration_test.go` | `TestPageMatrix` |
| `class-page/delete` | class page (admin) | `pages_integration_test.go` | `TestPageMatrix` |
| `class-page/get-list` | class pages (admin) | `pages_integration_test.go` | `TestPageMatrix` |
| `class-invite/create` | makes a link, token shown once (admin) | `invite_integration_test.go` | `TestInviteMatrix` |
| `class-invite/get-list` | links without tokens (admin) | `invite_integration_test.go` | `TestInviteMatrix` |
| `class-invite/revoke` | revokes a link (admin) | `invite_integration_test.go` | `TestInviteMatrix` |
| `class-invite/lookup` | class name and school for a live token (public) | `invite_integration_test.go` | `TestInviteDeadTokensLookAlike` |
| `class-invite/accept` | joins as student | `invite_integration_test.go` | `TestInviteAcceptMatrix` |

Outside `class*` but part of the privacy boundary:

| Endpoint | Why it matters | Test file | Test |
|---|---|---|---|
| `media/get-content` | the one read path into a student's photos (profile photo, approved notes' photos and, since T-152, design images, admin only; every other caller, another class's admin and an unlinked or removed member's book: 404) | `book_integration_test.go` | `TestAdminMediaReadPath` |
| every `yearbook/*`, `note/*`, `note-collection/*`, `media/*`, `book-data/*`, `book-export/*` route | an admin or student gets 404 on someone else's book | `book_integration_test.go` | `TestAdminRoutesAreAllInTheMatrix` |
| the same routes, called as the admin on a linked student book | still 404 | `page_integration_test.go` | `TestAdminStillHasNoRightsOverAStudentsBook` |

End to end: `tools/e2e/class.spec.mjs` (docs/e2e.md) runs the whole class journey in a browser and checks, over HTTP as student A, that B's yearbook, B's notes and the
class book are refused, and that a pending note never reaches the admin's printout.

## 7. Open points for the owner

1. Minors: the terms say 18 or older; school classes are often younger (section 1). **Decided (D-40, T-139):** the class create screen and the join screen say "SMemories is for people aged 18 or older." as plain text, not a blocker.
2. The note form does not tell a friend that the note may be seen by a class admin (section 1). **Decided (D-40, T-139):** one sentence follows the notice under the send button of the public friend note form.
3. ~~Invite links of a departed admin stay live until expiry or revocation~~ Decided (D-40 c): they are revoked with the admin role (section 4).
4. The legal texts are still drafts: the bracketed items (operator, contact, region, retention days, law) are open, and the Vietnamese needs a native review.
5. Printed or saved copies leave our control (section 3).

## 8. Owner sign-off

Left unchecked on purpose. Only the owner ticks these, in a later change, after reading sections 1 to 7 and the four texts in `docs/legal/`.

- [ ] I have read this review and the privacy policy and terms (version 2026-10-29), English and Vietnamese.
- [ ] I accept that a class admin sees the profile and the approved notes and photos of every student who linked a page, including the friends' names and messages.
- [ ] I decide whether the class feature is offered to people under 18, and what consent that needs (open point 1).
- [ ] I decide about a warning on the note form (open point 2) (open point 3, revoking a departed admin's links, is decided: D-40 c).
- [ ] The bracketed items of the legal texts are filled and the DRAFT banner is removed.
- [ ] The class feature may be used in production.

Signed: ____________________   Date: ____________
