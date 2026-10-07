# T-031 — Auth hardening for go-live: edge rate limits, shared limiter, session purge, stored-hash caps

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P3 · **Type:** security

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Items from the T-006 review that only matter at scale or in production: (1) the in-memory rate limiter is per process and keyed by unbounded client-chosen values; put a coarse limit at the edge (CloudFront/WAF) and move the counters to a shared store when more than one API task runs; (2) purge expired sessions on a schedule (today only a user's own login sweeps their rows); (3) lower the stored-hash memory cap in password verification (1 GiB today, a corrupted row could make a login allocate that much) to a few hundred MiB; (4) revisit login-CSRF (the origin check applies only to requests that carry the session cookie).

_BACKLOG: needs a full spec before it moves to TODO._

#### Leader notes from the T-007 review (2026-10-08)
The forgot-password limit of 3 per hour per email address (required by T-007 AC5) lets anyone block a victim's password reset for an hour by requesting it three times.
Accepted for M1; when the shared limiter lands, decide whether to keep it, to key it on (IP, email), or to answer with the same 202 but silently not send past the cap.
