# Legal texts: how to change them

`terms.en.md`, `terms.vi.md`, `privacy.en.md` and `privacy.vi.md` are the Terms of Use and the Privacy Policy (D-35). Each starts with a front matter whose `version`
(a date, for example `2026-10-21`) is the version users accept. The API keeps the current version in one constant, `legal.CurrentVersion` in `internal/legal/legal.go`.

To change a text:

1. Edit the text. If you change one language, change the other too.
2. Set the same new `version` in the front matter of **all four** files and in `legal.CurrentVersion`. `go test ./internal/legal` fails until they agree.
3. Update the version in the Postman collections (`termsVersion` variable) and, if you quote it, in the examples of `api/openapi.yaml`.

What happens next: `POST /api/auth/register` accepts only the new version; every signed-in user whose accepted version differs gets `ERROR_TERMS_REQUIRED` (403) from all
signed-in endpoints but `get-me`, `accept-terms`, `logout` and the email-verification ones, so each user re-accepts at their next request (the web shows the accept screen).
Notes written by friends keep the version that was current when they were submitted (`note_tab.terms_version`); users keep theirs in `user_tab.terms_version` and
`terms_accepted_at` (Unix ms).

Do not edit a published text without bumping the version: the record of who accepted what would no longer match the text.
