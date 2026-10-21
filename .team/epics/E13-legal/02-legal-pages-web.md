# E13_T-112 — Web: Terms and Privacy pages, sign-up checkbox, accept screen, friend-form notice

**Epic:** E13-legal · **PRD:** [PRD.md](PRD.md) · **Milestone:** M2 · **Risk:** low · **Priority:** P2 · **Type:** feature
**Depends on:** T-111. **Read first:** the PRD, `docs/legal/*.md`, the Register page, the session bootstrap (`get-me`), the public note form, the i18n parity check.

#### Requirements (SHALL)
1. **Pages.** Public routes `/terms` and `/privacy` show the document in the UI language (en or vi), with its version and date. One source of truth: the files in `docs/legal/` (import them at build time, for example Vite `?raw` with `server.fs.allow`, or a copy step checked by a test; say which). The files use only headings, paragraphs, bullet lists and bold: render them with a small local renderer that builds React elements (no `dangerouslySetInnerHTML`, no new dependency). While the front matter `status` starts with `DRAFT`, show a visible banner "Draft: not the final text" / "Bản nháp: chưa phải bản chính thức".
2. **Sign-up.** Register has a required checkbox "I agree to the Terms of Use and the Privacy Policy" / "Tôi đồng ý với Điều khoản sử dụng và Chính sách quyền riêng tư" with both titles linking to the pages (new tab). The button stays disabled until it is ticked; the request sends `accept_terms_version` (the version of the docs).
3. **Accept screen.** When get-me says `terms.required` (or any call answers `ERROR_TERMS_REQUIRED`), the app shows `/accept-terms`: title "Our terms have changed" / "Điều khoản đã được cập nhật" (for a first Google sign-in: "Before you start" / "Trước khi bắt đầu"), links to both pages, the same checkbox, buttons "Accept and continue" / "Đồng ý và tiếp tục" and "Sign out" / "Đăng xuất". After accepting, the user returns where they were going.
4. **Links.** Terms and Privacy links in the footer of public pages and in the signed-in shell.
5. **Friend form notice.** Under the send button of the public note form, one line: "By sending, you agree to the Terms of Use and the Privacy Policy. The book owner may share your photos by link." / "Khi gửi, bạn đồng ý với Điều khoản sử dụng và Chính sách quyền riêng tư. Chủ sổ có thể chia sẻ ảnh của bạn qua liên kết." with links.
6. **E2E.** The journey ticks the checkbox at sign-up; add one step: an existing user with an old version is sent to the accept screen and continues after accepting.

#### Acceptance criteria
- [ ] AC1 — Component tests: checkbox gating, sent version, accept screen flow (accept, sign out), renderer escapes text and handles the four constructs, draft banner shown and hidden by status.
- [ ] AC2 — Real-browser screenshots at 375 px and 1280 px of both pages (en and vi), Register, the accept screen and the friend form (in the PR).
- [ ] AC3 — i18n parity, lint, `make e2e` green.

#### Risk
`low`: web only; the gate is T-111.
