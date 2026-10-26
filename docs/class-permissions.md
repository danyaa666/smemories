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
| `admin` | the teacher or the class monitor | creating the class, or being promoted by an admin |
| `student` | everybody else in the class | joining with an invite (T-125), or being demoted by an admin |

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
| Create, list, revoke invites (T-125) | yes | 403 | 404 | 404 | 401 |
| Join with an invite (T-125) | n/a | yes, becomes student | needs an account first | yes, becomes student | 401 |
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
   the display name, role, join time and whether a class page is chosen (`page_linked`); no email, no user id, no page content.
6. **Students never see the roster.** `class/get` returns the class and the caller's own role only, for every role.
7. **Deleting is local.** Deleting a class removes the class and membership rows and nothing else; a member's yearbook is never touched.
   Deleting a yearbook clears the link in `class_member_tab.yearbook_id` and keeps the membership. Leaving or being removed never touches
   the member's yearbook.
8. **Verified email to create.** `class/create` needs a verified email (403 `ERROR_EMAIL_NOT_VERIFIED`); a user can be admin of at most 10
   classes (409 `ERROR_LIMIT_REACHED`).
9. **Writes are rate-limited** per user (120 an hour, shared by the class writes) and fail closed: if the limiter store is down, the
   write is refused with 503 `ERROR_UNAVAILABLE`. Reads are not limited.

## Limits

Classes per user as admin: 10. Members per class: 60 (enforced where members are added, T-125). Active invites per class: 5 (T-125).
Class pages per class: 30 (T-128). A class name is 1 to 120 characters; the school 0 to 120; the graduation year 1950 to 2100;
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
`internal/db/class_tab_integration_test.go` (migration round trip and the keys), and `postman/class.postman_collection.json` (the
same flows over HTTP, newman).
