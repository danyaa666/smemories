# E13 — Terms of Use and Privacy Policy

**Status:** planned   **Milestone:** M2 (before go-live)   **Owner decisions:** D-35 (README §4), D-01 (18+ owners), D-32 (public photo links)

## Problem
SMemories stores friends' personal data and photos and, since D-32, can expose photos through public links. Nobody has agreed to anything: there are no terms, no privacy policy and no record of consent.

## Goal and non-goals
- Goal: a Terms of Use and a Privacy Policy page in English and Vietnamese; a required, versioned acceptance at sign-up (password and Google) with re-acceptance when the text changes; a notice on the public friend form; a record of which version each user and each note was given.
- Non-goals: cookie banner (only one essential cookie, no tracking), age verification beyond the 18+ statement in the terms (owner chose no extra confirmation), legal review (owner's job before go-live), account deletion and export (T-025).

## The texts
Drafts written by the leader from what the product actually does: `docs/legal/terms.{en,vi}.md`, `docs/legal/privacy.{en,vi}.md`, version `2026-10-21`, status DRAFT. Every `[bracketed]` item (operator, contact, law, region, retention days) is the owner's to fill. Go-live checklist (E06): texts reviewed, brackets filled, status set to final, version bumped.

## Tasks
| Task | Spec | Depends on |
|---|---|---|
| T-111 Versioned terms acceptance (API) | `01-terms-acceptance-api.md` | — |
| T-112 Web: legal pages, sign-up checkbox, accept screen, friend-form notice | `02-legal-pages-web.md` | T-111 |
