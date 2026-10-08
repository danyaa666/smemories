# T-031 — Auth hardening for go-live: edge rate limits, shared limiter, session purge, stored-hash caps

**Epic:** E02-auth · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** high · **Priority:** P3 · **Type:** security

<!-- Migrated from the board block on 2026-10-07; the text below is unchanged. The board keeps status, dependencies and comments only. -->

#### Intent
Items from the T-006 review that only matter at scale or in production: (1) the in-memory rate limiter is per process and keyed by unbounded client-chosen values; put a coarse limit at the edge (CloudFront/WAF) and move the counters to a shared store when more than one API task runs; (2) purge expired sessions on a schedule (today only a user's own login sweeps their rows); (3) lower the stored-hash memory cap in password verification (1 GiB today, a corrupted row could make a login allocate that much) to a few hundred MiB; (4) revisit login-CSRF (the origin check applies only to requests that carry the session cookie).

_BACKLOG: needs a full spec before it moves to TODO._

#### Leader notes from the T-007 review (2026-10-08)
The forgot-password limit of 3 per hour per email address (required by T-007 AC5) lets anyone block a victim's password reset for an hour by requesting it three times.
Accepted for M1; when the shared limiter lands, decide whether to keep it, to key it on (IP, email), or to answer with the same 202 but silently not send past the cap.

#### Leader notes from the T-011 review (2026-10-08)
The Google start and callback limits (30 per 15 minutes per client IP, T-011 AC8) and the per-IP register/login limits are per source address. Students on one campus network often share a single public IP
(NAT): a class signing in together could hit 429. Before go-live, make these limits configurable, measure against a realistic burst (for example 60 students in 5 minutes) and decide whether to key on IP + user agent
class or move the first line of defence to the edge (WAF rate rules). Also: the OIDC discovery fetch holds a lock for up to 5 s (ponytail in `discover`); use singleflight so a Google outage cannot queue requests.

The same shared-address concern applies to the public collection lookup (T-012, 60 per 15 minutes per IP): see the T-034 notes; include it in the burst measurement.

The public note submission (T-034) limits per client IP are 100 per hour and 300 per day, and 60 submissions per collection per hour (constants in `internal/notes/submit.go`). A class link posted in a group chat can see more than 60 real submissions in its first hour (a collection holds at most 300 notes in total); in the burst measurement decide whether to raise the per-collection hourly limit to about 150.

#### Leader note from decision D-23 (2026-10-08)
The "shared limiter" and "session purge" items of this task are done by T-053 and T-052 (Redis); drop them from this task's scope when it is promoted. Remaining: edge limits, stored-hash caps, the limit-tuning measurements above.
