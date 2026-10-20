# E12 — Security hardening from the 2026-10-20 audit

**Status:** planned   **Milestone:** M2 (before the first deploy, T-023)   **Source:** [docs/security-audit-2026-10-20.md](../../../docs/security-audit-2026-10-20.md), leader decision L-15.

## Problem
The audit found no critical or high issue, but four gaps that are cheap to close now and expensive to find in production: rate limits that an IPv6 host can walk around and no per-account login cap (M1), a process that silently boots in `dev` mode when `SMEM_ENV` is missing (M2, L7), client-chosen spelling of public ids that lets a user orphan S3 objects and defeat deletion (M3), and a few small items (L1-L3).

## Goal and non-goals
- Goal: every audit finding that is code is fixed or has a named owner, before T-023 deploys anything.
- Non-goals: WAF and edge rate limits (T-031), the per-user session cap (T-031), edge headers and log hygiene (added to the T-023 spec), GitHub settings and the agent identity (owner questions on the board), sign-out-everywhere (a new endpoint and screen; add when the Account page needs it, the `DeleteAll` session script already exists).

## Tasks (specs in this folder)
| Task | Spec | Audit item | Depends on |
|---|---|---|---|
| T-098 IPv6 /64 keying and a per-account login limiter | `01-ipv6-keying-and-account-login-limit.md` | M1 | — |
| T-099 Mandatory `SMEM_ENV` and local-only dev/test | `02-mandatory-env-and-local-only-dev-test.md` | M2, L7 | — |
| T-100 Canonical public ids and the stored id for object keys | `03-canonical-public-ids.md` | M3 | — (before T-025) |
| T-101 Small hardening: readyz cache, generic code-email greeting, absolute session lifetime | `04-readyz-greeting-session-lifetime.md` | L1, L2, L3 | — |

## Design decisions (L-15)
- **Per-account login limit is a trade-off, accepted.** Counting failures per account lets an attacker lock a victim's *login* for up to 15 minutes by failing 50 times. It cannot touch password reset or Google sign-in, and the limit is high enough that only automation hits it. The alternative (no per-account cap) leaves guessing bounded only by argon2 cost.
- **Validate at the edge and use the stored id.** Changing the `public_id` collation to a binary one needs a table rebuild on a pre-production database for no extra safety once handlers reject non-canonical ids and services build keys from the stored id.
- **`dev` and `test` are local-only.** The process refuses them when an allowed origin or the public base URL is not localhost or a private address, so a staging deployment cannot run with the public OTP key or the fixed code by picking `test`.

## Risks
- T-098 and T-100 change behaviour that clients can see (a lower-case id becomes 400; /64 sharing may rate-limit two students on one campus prefix, the NAT concern already noted in T-031). There are no production users yet; the web app only sends ids it received from the API.
- T-099 makes every command fail without `SMEM_ENV`: `.env.example`, `Makefile`, scripts and tests must all set it.
