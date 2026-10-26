# Class permissions (E07, D-36)

The written contract of who may do what in a class. It is the matrix of the class yearbook design (T-026), enforced in code by
`class.Service.Authorize` and tested endpoint by endpoint in `internal/class/matrix_integration_test.go`. Every later E07 endpoint
(invites, the student page link, the class book, class pages) extends this table and its test; none may widen it.

## The idea in one paragraph

A student's page in the class book is the student's own personal yearbook. A class is a thin layer on top of it: `class_tab` (the
space) and `class_member_tab` (who is in it, with a role, and which of their yearbooks is their class page). **Joining a class gives
nobody any right over anybody's yearbook.** The ownership checks of `yearbook`, `notes` and `media` (`owner_id = ?`) are unchanged: an
admin never edits, moderates or reads the unapproved content of a student's book.

## Roles

| Role | Who | How it is obtained |
|---|---|---|
| `admin` | the teacher or the class monitor | creating the class, or being promoted by an admin; never by an invite |
| `student` | everybody else in the class | joining with an invite, or being demoted by an admin |

A class always has at least one admin. Roles are per class: the same person can be admin of one class and student of another.

## The matrix

| Action (endpoint) | Admin | Student (member) | Member of another class | Signed-in non-member | Anonymous |
|---|---|---|---|---|---|
| Create a class (`class/create`, verified email) | 200, becomes admin | 200, own new class | 200, own new class | 200, own new class | 401 |
| List my classes (`class/get-list`) | 200, own classes only | 200, own classes only | 200, own classes only | 200, empty | 401 |
| See class name, my role, my page status (`class/get`) | 200 | 200 | 404 | 404 | 401 |
| Change class details (`class/update`) | 200 | **403** | 404 | 404 | 401 |
| Delete the class (`class/delete`) | 200 | **403** | 404 | 404 | 401 |
| See the roster (`class-member/get-list`) | 200 | **403** | 404 | 404 | 401 |
| Promote or demote a member (`class-member/update-role`) | 200 (last admin protected) | **403** | 404 | 404 | 401 |
| Remove a member (`class-member/remove`) | 200 (last admin protected) | **403** | 404 | 404 | 401 |
| Leave the class (`class-member/leave`) | 200 unless last admin | 200 | 404 | 404 | 401 |
| Link or clear **my own** page (`class-member/set-book`; T-126) | 200, own yearbook only | 200, own yearbook only | 404 | 404 | 401 |
| Create, list, revoke invites (`class-invite/create`, `get-list`, `revoke`; T-125) | 200 | **403** | 404 | 404 | 401 |
| Preview an invite link (`class-invite/lookup`, public; T-125) | 200 for a live token, else 404 | same | same | same | same (no session needed) |
| Join with an invite (`class-invite/accept`, verified email; T-125) | 200, keeps `admin` | 200, keeps `student` | 200, becomes student | 200, becomes student | 401 |
| Read class book data: profiles, approved notes, class pages (T-126) | yes | **no (403)** | 404 | 404 | 401 |
| Read a student's pending or hidden notes, note-link tokens, unapproved photos | **never** | own only, via the normal yearbook endpoints | no | no | no |
| Edit a student's yearbook | **never** | own only | no | no | no |
| Edit class pages, order the book (T-128) | yes | 403 | 404 | 404 | 401 |

Rows marked with a ticket id are enforced by that ticket; this file is the contract they implement.

## Rules behind the table

1. **No existence leak.** "No such class" and "not a member" are the same answer: HTTP 404 `ERROR_NOT_FOUND`, `class not found`, byte for
   byte identical apart from `request_id`. A member whose role is too low gets 403 `ERROR_FORBIDDEN`. (A member who may know the class
   exists learns nothing new from a 403.) The test asserts the equality for the non-member and for a member of another class.
2. **Every check is SQL on `class_member_tab`.** `Authorize(ctx, userID, classPublicID, minRole)` is one join of `class_member_tab`
   (the caller's user id and role) with `class_tab`. Nothing is decided from an earlier read or a cached role. Endpoints that change
   membership repeat the check under a lock inside their transaction (the class row `FOR UPDATE`, then the caller's member row), so a
   caller removed or demoted a moment ago is refused.
3. **Anonymous is 401, not 404.** Every class endpoint sits behind `RequireUser`, so a request without a session never reaches a class
   lookup and learns nothing about classes (the answer is the same whether the id exists or not).
4. **The last admin stays.** The last admin can neither leave, be removed nor be demoted (403 `ERROR_LAST_ADMIN`); the way out is to
   promote someone else first, or delete the class. The rule holds under concurrency: all membership changes of one class take the
   class row lock first, so two admins leaving at once leave exactly one.
5. **Members are named by an opaque member id** (`class_member_tab.public_id`, a ULID), never by the user id or email. The roster shows
   the display name, role, join time, whether a class page is chosen (`page_linked`), and two numbers about it, `profile_complete` and `approved_notes`
   (T-126, below); no email, no user id, no page content.
6. **Students never see the roster.** `class/get` returns the class and the caller's own role only, for every role.
7. **Deleting is local.** Deleting a class removes the class and membership rows and nothing else; a member's yearbook is never touched.
   Deleting a yearbook clears the link in `class_member_tab.yearbook_id` and keeps the membership. Leaving or being removed never touches
   the member's yearbook.
8. **Verified email to create or join.** `class/create` and `class-invite/accept` need a verified email (403 `ERROR_EMAIL_NOT_VERIFIED`).
   A user can be admin of at most 10 classes (409 `ERROR_LIMIT_REACHED`), whether by creating one or by being promoted in one
   (`class-member/update-role`): the count is taken under a lock on the user's row, so two classes promoting the same person, or a promotion
   racing a `class/create`, cannot both take the last slot.
9. **Writes are rate-limited** per user (120 an hour, shared by the class writes) and fail closed: if the limiter store is down, the
   write is refused with 503 `ERROR_UNAVAILABLE`. Reads are not limited.

## Invites (T-125)

An invite is a link `<public base URL>/join/<token>` that an admin makes and shares by hand (no invite email, docs/email.md). Whoever holds a
live link may join as a `student`; the link is a bearer secret, so:

1. **The token is shown once.** 32 random bytes (`crypto/rand`), base64url, 43 characters. `class-invite/create` returns it and the link;
   the server stores only the SHA-256 (`class_invite_tab.token_hash`, unique), so nothing can show it again, and `get-list` has no token.
2. **It is never logged.** Handlers read it from the JSON body (`POST` for `lookup` and `accept`), not from a URL; the API's access log
   records the route only; no error, metric label or stored column holds it. The web path `/join/<token>` is redacted wherever a
   URL of the web app is recorded: the Playwright trace scrubber (`tools/e2e/scrub-trace.sh`) and, for T-023, the edge (ALB / CloudFront /
   web-server) access logs, which must drop or mask the path after `/join/` exactly as they must for `/c/`. A test greps a run's logs for the token.
3. **Dead tokens look alike.** Unknown, malformed, empty, expired and revoked tokens get the same 404 `ERROR_NOT_FOUND` (`invite not found`),
   byte for byte, on `lookup` and on `accept`, also for a caller who already belongs to the class. All take the same code path (one lookup by
   hash, then a constant-time compare of the hash); the state is judged afterwards.
4. **Lookup shows the class name and school** and nothing else (not the member count, not whether the class is full). Rate limits per
   client IP: 600 requests and 20 misses per 10 minutes (a live link is not a miss), fail open because a 256-bit token cannot be guessed.
5. **Accept is a write and is idempotent.** It counts against the user's 120 writes an hour and against 300 accepts per IP an hour (a wrong
   token costs a write too), fails closed if the limiter store is down, and never changes an existing member's role: an admin who opens
   an invite stays admin, a promoted student stays promoted. A new member is always a `student`.
6. **Caps are checked under the class row lock** (the same lock as every membership change): 5 active (unrevoked, unexpired) invites,
   60 members. A full class answers 409 `ERROR_CLASS_FULL` (a member of a full class still gets their membership back); ten accepts of the
   last seat at once add exactly one member. A revoke takes the same lock, so an accept that starts after a revoke commits is refused.
7. **Lifetime** 14 days by default; `expires_at` (Unix ms) may be set from now (exclusive) to 30 days.
8. Revoking a link removes nobody who joined with it; removing a member does not revoke the link. Deleting a class deletes its invites.
9. An invite or member id is looked up inside the class named in the request, so an admin of class A who sends an id of class B gets 404
   and nothing changes (matrix test `TestCrossClassIDsChangeNothing`).

## The student's page (T-126)

A member picks which of their own yearbooks is their page in the class book with `class-member/set-book` (`{class_id, yearbook_id}`; `yearbook_id: null`
clears it, an absent key is refused so a typo cannot unlink). The link is a pointer in `class_member_tab.yearbook_id` and nothing else.

1. **Only the owner, only their own membership.** The caller is always the member: the body cannot name another member (an unknown field is
   400), so an admin cannot set or clear a student's link. The yearbook is looked up as `public_id = ? AND owner_id = <caller> FOR UPDATE`, the
   same ownership check as everywhere: someone else's yearbook and a missing one are the same 404 (`yearbook not found`). A caller who is not a
   member gets the class 404 first, whichever yearbook they send.
2. **One yearbook, at most one class.** Linking a yearbook that is already the page of another membership (of the same user in another class)
   is 409 `ERROR_CONFLICT`; the check runs under the yearbook row lock and the unique key `uq_class_member_tab_yearbook` is the backstop.
   Setting the same link again is a no-op 200; setting another yearbook replaces the link.
3. **Lock order: yearbook row, class row, member row.** A yearbook delete takes the yearbook row and then clears the member row
   (`ChildPurger`); `set-book` takes the same order, so a link racing a delete either loses (404, the book is gone) or is cleared by the
   delete in the same transaction. No member row ever points at a missing yearbook. The rest of the class code never takes a yearbook
   row, so there is no cycle.
4. **Linking changes no permission.** Every `yearbook/*`, `note-collection/*`, `note/*`, `media/*`, `book-data/*` and `book-export/*` endpoint
   still checks `owner_id = caller`, so the class admin gets 404 on all of them for a student's linked book (tested endpoint by endpoint in
   `page_integration_test.go`); the one exception is `media/delete`, which by design answers 200 and does nothing for a photo that is not the
   caller's, so repeating a delete is harmless and nothing leaks. Leaving the class or being removed never touches the yearbook; deleting the yearbook keeps the membership.
5. **What the member sees** (`class/get`, and the answer of `set-book`): `page: {linked, yearbook_id, profile_complete, approved_notes}` for their
   own row only. **What the admin sees** (`class-member/get-list`): per member `page_linked`, `profile_complete`, `approved_notes` and nothing
   else: no profile text, no note text, no photo id, no yearbook id, and not the pending or hidden count. A link to a yearbook the member does not
   own (the API cannot make one) counts as not linked.
6. **Definitions.** `profile_complete`: the owner profile of the linked yearbook has a non-empty name and a photo. `approved_notes`: the notes
   of that yearbook in status `approved`. Both are 0/false while nothing is linked. The roster reads them for the whole page in two grouped
   queries, whatever the page size.

## Client IP behind the proxy

Per-IP limits (invite lookup and accept, and the other public endpoints) key on `httpx.ResolveClientIP`. By default it is the host of
`RemoteAddr`. With `SMEM_TRUST_PROXY=true` it is the **last** `X-Forwarded-For` hop, the one our own load balancer appended (earlier hops are
client-controlled), and IPv6 is keyed by its /64. In production the API must be reachable only through the ALB with `SMEM_TRUST_PROXY=true`;
otherwise every student shares the ALB's address and the limits lock the whole class out together (or, with the flag on and the task reachable
directly, the address can be forged). See [production-config.md](production-config.md#client-ip-and-smem_trust_proxy); T-023 must set the flag and
keep the task behind the ALB only. A class of 60 on one school network (one NAT address) therefore shares one budget, which is why the invite
budgets are generous (600 lookups per 10 minutes, 300 accepts per hour; only the miss limiter of 20 per 10 minutes is tight).

## Limits

Classes per user as admin: 10 (create and promotion). Members per class: 60 (`class-invite/accept`). Active invites per class: 5.
Invite lifetime: 14 days by default, 30 at most. Class pages per class: 30 (T-128). A class name is 1 to 120 characters; the school 0 to 120; the graduation year 1950 to 2100;
the language `en` or `vi`.

## Privacy statement

What the class layer stores about a person: that they are a member of a class, with which role, since when, and which of their own
yearbooks they chose as their class page. Admins see the roster (display names, roles, whether a page is chosen). Students see only
their own row. Nothing in a student's yearbook becomes visible to an admin through this layer; the only content that will ever reach a
class admin is what the student has already approved for the book (T-126, class-book/get), read-only and scoped to the class. A person
who leaves, or is removed, stops being visible to the class at once and keeps their yearbook. The legal texts are updated for class data
in T-131, which also has the owner sign off before production use.

## Where it is tested

`internal/class/matrix_integration_test.go` (the matrix above, one fresh class per cell, plus the 404 equality), `class_integration_test.go`
(last admin under concurrency, caps, validation, canonical ids, paging, rate limit, deleting a class or a yearbook),
`page_integration_test.go` (the student's page: set-book matrix, one yearbook one class, link racing delete, roster keys, the admin's 404 on the whole book stack, 60 students from one IP),
`invite_integration_test.go` (invites: matrix cells, caps, concurrency, dead tokens, the log grep, the cross-class ids and the promotion cap),
`internal/db/class_tab_integration_test.go` (migration round trips and the keys), and `postman/class.postman_collection.json` (the
same flows over HTTP, newman).
