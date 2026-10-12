# E04_T-080 — Public submit: show "link full" instead of a connection reset

**Epic:** E04-friends-notes · **PRD:** [PRD.md](PRD.md) · **Milestone:** M1 · **Risk:** low · **Priority:** P3 · **Type:** tech-debt

#### Description
T-068 moved the "link is full" check (`ERROR_COLLECTION_FULL`, 409) in front of the body spool, as in T-034, and answers it on the raw writer **without draining the body** (an unknown token is answered the same way, on purpose). For an unknown token that is right: nobody legitimate is on the other end. For a **full link** the sender is a real friend with a valid token who may be mid-upload of up to 32 MiB of photos: closing the connection with unread data can reach the browser as a network error (connection reset) instead of the 409 message, so the friend sees a generic failure and not "this link is full".

#### Scope
- In: `internal/notes` (submit path for `ERROR_COLLECTION_FULL`, and for the expired/closed link if it behaves the same). The web side is a note in T-018's spec.
- Out (do not do): spooling the body, changing caps or pacing of T-034, changing the unknown or revoked token path (still raw writer, body unread).

#### Requirements (SHALL)
1. A request with a **valid** token to a full link SHALL be answered 409 `ERROR_COLLECTION_FULL` in a way the browser can read: either drain the unread body to `io.Discard` under the existing pace limits and size cap (no spool file, no disk write) before replying, or reply and close with the client able to finish reading the response. Pick the smaller change and say which in the PR.
2. The drain SHALL be bounded by the same idle/rate/size limits as a normal upload (a client that stalls is dropped with 408, as today), and SHALL write nothing to disk.
3. The unknown, revoked and malformed token paths SHALL stay unchanged: 404 on the raw writer, body unread.

#### Acceptance criteria
- [ ] AC1 — Integration test over a real socket: a valid token on a full link, 20 MiB body sent: the client reads a 409 `ERROR_COLLECTION_FULL` envelope (not a reset), spool dir empty, no file written, and the server did not hold the connection beyond the pace limits.
- [ ] AC2 — Unknown token with a 20 MiB body is still answered 404 with 0 bytes read (T-068's test stays green).
- [ ] AC3 — The route's slow-client protections (idle 408, rate 408) still apply to the drained body.

#### Test plan
- QA should probe: a full link plus a stalled uploader, a full link plus a 40 MiB Content-Length (413 must still win), the honeypot on a full link (409, never stored).
