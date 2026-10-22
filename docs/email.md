# Transactional email (T-021, D-29)

The API sends two kinds of mail, both a 6-digit code the student types (no links, no tracking): the **verification** code and the **password reset** code
([auth-otp.md](auth-otp.md)). Production sends them through **Amazon SES** (SESv2 `SendEmail`) from a verified subdomain; development prints them.

## How the application sends

- The application depends only on `mailer.Mailer`, `mailer.Message{To, Subject, Text, HTML}` and two errors, `mailer.ErrUnavailable` (throttled, 5xx, timeout, network: try
  later; also an account or setup problem only the operator can fix, with the class `config`) and `mailer.ErrRejected` (the provider refuses this one address for good). Nothing outside `internal/mailer/ses.go` imports an AWS mail package or knows an SES error name.
- `mailer.New(cfg, logger)` is the only place that picks the implementation from `SMEM_MAIL_PROVIDER` (`log` or `ses`). Settings: [production-config.md](production-config.md#email-amazon-ses).
- Every message has a plain-text and an HTML part (multipart/alternative), UTF-8. The HTML is small (about 1.3 KB), has one `h1`, a `lang` attribute, no remote resource and no link
  (`internal/auth/emails/`; golden files in `internal/auth/testdata/emails/`, open the `.html` ones in a browser to see them; `go test ./internal/auth -run Golden -update` rewrites them).
- The greeting is generic (`Hello,` / `Xin chào,`): the display name is typed before the address is proven, so it never appears in a code email (audit L2, T-101).
- SES client: region and credentials from the AWS default chain (`AWS_REGION`, the ECS task role), dial 3 s, TLS 5 s, response headers 15 s, the whole call 30 s, at most 3 attempts with a
  backoff capped at 2 s. The caller's own deadline (10 s) wins when shorter.

| Where | How it sends | If the provider fails |
|---|---|---|
| `POST /api/auth/register` | background, never waits | up to 2 retries (2 s, 5 s) while `ErrUnavailable` (none for class `config`: a misconfiguration does not fix itself in seconds); then one ERROR log line (`request_id`, `class`, no address, no code) and `smem_mail_send_failures_total{class}`. The account exists; the student uses "send a new code". |
| `POST /api/auth/forgot-password` | background; the answer is the same `200` for every address, known, unknown or refused | same as above |
| `POST /api/auth/resend-verification` | on the request, once, at most 10 s | `ErrUnavailable` -> `503 ERROR_MAIL_UNAVAILABLE` + `Retry-After: 30` (the resend try is given back, but at most 5 times an hour per user, so an outage cannot be used to hammer the provider: after that the tries count as usual, at most 5 + 3 sends an hour, then `429`); `ErrRejected` -> `400 ERROR_EMAIL_UNDELIVERABLE` |

Failure classes (the metric label): `unavailable`, `rejected`, `config`, `shutdown`, `other` (a misconfiguration or bad credentials, which is also an ERROR log line). Alarm on
`rate(smem_mail_send_failures_total[15m]) > 0` for `unavailable`, `config` and `other` (T-024); `config` must page at once (every send fails until the operator acts); the metric counts the background sends only (register, forgot-password), so T-024 must also alarm on the ERROR log line `mailer: send failed` with `class=config` (T-091), which covers the request-path resend failures too; `rejected` is the
student's address and only needs watching in bulk; `shutdown` counts emails dropped because the process had to exit before they were sent ([container.md](container.md#graceful-shutdown)),
a few after a deploy are normal, a steady stream means the provider is slow.

SES errors and what they become (the one place that knows them is `classifySES` in `internal/mailer/ses.go`): `AccountSuspendedException`, `SendingPausedException` and a
`MessageRejected` whose text says an identity "is not verified" (an unverified sender, or in the sandbox an unverified recipient) -> `ErrUnavailable` with class `config`
(the student gets the 503 "try again shortly", the log line is ERROR with the SES error code and `not verified`, never the text); any other `MessageRejected` (this address
is refused for good) -> `ErrRejected`; `TooManyRequestsException`, `LimitExceededException`, other throttling codes, any 429 or 5xx, a timeout and a connection that never got an answer -> `ErrUnavailable`;
everything else (for example `NotFoundException` for a wrong configuration set, `MailFromDomainNotVerifiedException`) -> class `other`.

**Privacy.** The recipient address, the code and the body are never logged and never put in an error text (SES error messages contain the address, so `Error()` carries only the class and the
SES error code). The tests assert it on the SES stub and for the log mailer.

## Day-one symptom: the SES sandbox

A new SES account is in the **sandbox**: it delivers only to verified addresses and only from a verified sender identity. Until both are done, every send is refused with
`MessageRejected: Email address is not verified`. The API reports this as class `config` (503 on the resend screen, an ERROR log line `mailer: send failed ... class=config reason="MessageRejected not verified"`
and `smem_mail_send_failures_total{class="config"}`), not as "undeliverable address". The order to fix it: (1) verify the sender identity (step 1 to 3 below) and check
`aws sesv2 get-email-identity`, (2) in the sandbox verify your own test recipient (step 8), (3) leave the sandbox (step 7), then go live. `AccountSuspendedException` and
`SendingPausedException` are class `config` too: look at the SES console (Account dashboard) and the AWS Health events; they are not fixed by retrying.

## Bounces and complaints (no webhook)

No SNS endpoint is built: a public endpoint receiving signed messages is more attack surface than this volume justifies. Instead:

1. Turn on the **account-level suppression list** for `BOUNCE` and `COMPLAINT` (step 5 below). SES then stops delivering to an address that hard-bounced or complained.
2. **Check this in the sandbox before launch.** AWS documents that a send to a suppressed address is *accepted and dropped* (it is recorded as a suppressed bounce), not refused with an
   API error. If that is what you see, the student of a suppressed address gets "we sent a code" and never receives it; the recovery is the resend screen, and the address shows in the
   suppression list (`aws sesv2 list-suppressed-destinations`). If SES does refuse with `MessageRejected`, the API answers `400 ERROR_EMAIL_UNDELIVERABLE` as designed. Record which one you saw.
   **Result of the sandbox test with `suppressionlist@simulator.amazonses.com`: not recorded yet.** It needs an SES account, which the owner has not created (decision D-31, domain and
   AWS work happen in the go-live phase); T-090 item 9 stays open until then. When it is known, write the result here and, if SES accepts and drops the send, decide whether the
   verification screen needs a "change my email" path (a new task, not part of T-090).
3. Reputation alarms (bounce rate, complaint rate) are CloudWatch alarms on the configuration set, built in T-024. SES reviews an account at 5% bounces or 0.1% complaints and can pause it at
   10% / 0.5%, which is why the alarms start at 2% / 0.05%.

## Owner-assisted setup (domain, DNS, sandbox exit)

These steps need an AWS account and DNS access; nothing in the code depends on having done them. Replace `mail.example.com` with the sending subdomain the owner picks (Q-037),
`eu-west-1` with the production region, `ACCOUNT` with the AWS account id. Use a **subdomain** (`mail.`), never the apex: reputation and DMARC policy then stay separate from the website.

1. **Create the domain identity** (region of the API):
   `aws sesv2 create-email-identity --region eu-west-1 --email-identity mail.example.com --dkim-signing-attributes NextSigningKeyLength=RSA_2048_BIT`
2. **Easy DKIM**: the call above returns three tokens. Publish three CNAME records in DNS: `<token>._domainkey.mail.example.com  CNAME  <token>.dkim.amazonses.com`.
   Status turns `SUCCESS` within minutes to 72 hours (`aws sesv2 get-email-identity --email-identity mail.example.com`, field `DkimAttributes.Status`).
3. **Custom MAIL FROM domain** (so SPF aligns with the From domain for DMARC):
   `aws sesv2 put-email-identity-mail-from-attributes --email-identity mail.example.com --mail-from-domain bounce.mail.example.com --behavior-on-mx-failure USE_DEFAULT_VALUE`, then publish
   - `bounce.mail.example.com  MX  10 feedback-smtp.eu-west-1.amazonses.com`
   - `bounce.mail.example.com  TXT  "v=spf1 include:amazonses.com ~all"`
4. **DMARC** at `_dmarc.mail.example.com`, a TXT record, starting in monitoring mode:
   `v=DMARC1; p=none; rua=mailto:dmarc-reports@example.com; adkim=r; aspf=r`
   (use a mailbox you read, or a DMARC report service). After 2 to 4 weeks of reports showing only your own SES traffic passing, move to `p=quarantine; pct=25`, then `pct=100`; `p=reject`
   is optional afterwards. Never skip `p=none`: a mistake in a stricter policy silently drops the verification mail.
5. **Account-level suppression list** for bounces and complaints:
   `aws sesv2 put-account-suppression-attributes --region eu-west-1 --suppressed-reasons BOUNCE COMPLAINT`
6. **Configuration set** with reputation metrics (the name goes in `SMEM_SES_CONFIGURATION_SET`):
   `aws sesv2 create-configuration-set --region eu-west-1 --configuration-set-name smem-prod --reputation-options ReputationMetricsEnabled=true --sending-options SendingEnabled=true --delivery-options TlsPolicy=REQUIRE`.
   Do **not** enable open or click tracking: the mails contain no links, and a tracking pixel would break the "no remote images" rule.
7. **Leave the SES sandbox** (support case "Service limit increase > SES sending limits" in the console, or `aws sesv2 put-account-details`). Ready-to-paste text:

   > Mail type: Transactional. Website URL: https://app.example.com. We run SMemories, a yearbook web app for school classes. We send exactly two kinds of email, both one-time
   > 6-digit codes the user asked for in the app: an email-verification code after sign-up (or when the user presses "send a new code") and a password-reset code. There are no
   > newsletters, marketing, links, attachments or tracking. Expected volume: under 2,000 emails a day at peak (class sign-up season), under 20,000 a month; a user can request at most 3
   > codes an hour, and sign-ups are limited per IP address, so the volume cannot spike without us noticing. Recipients are people who typed their own address into our sign-up form.
   > Bounces and complaints: we use the SES account-level suppression list for bounces and complaints, so SES stops sending to an address that bounced or complained; our sender domain
   > has DKIM, a custom MAIL FROM domain with SPF, and DMARC; we monitor the reputation metrics of a configuration set with CloudWatch alarms at 2% bounce and 0.05% complaint rate.
   > The API treats a provider refusal as "undeliverable address" and tells the user, and it never retries a refused address.

8. **Test while in the sandbox**: SES only delivers to **verified** recipient addresses. Verify your own: `aws sesv2 create-email-identity --email-identity you@gmail.com` and click the link
   in the mail SES sends. The mailbox simulator accepts any number of sends and never hurts your reputation: `success@simulator.amazonses.com`, `bounce@simulator.amazonses.com`,
   `complaint@simulator.amazonses.com`, `suppressionlist@simulator.amazonses.com` (use the last one to record the behaviour asked for in "Bounces and complaints", point 2).
9. **IAM** for the ECS task role (T-023 creates it): least privilege, `ses:SendEmail` only, on the identity and the configuration set, and only with the one From address:

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [{
       "Effect": "Allow",
       "Action": "ses:SendEmail",
       "Resource": [
         "arn:aws:ses:eu-west-1:ACCOUNT:identity/mail.example.com",
         "arn:aws:ses:eu-west-1:ACCOUNT:configuration-set/smem-prod"
       ],
       "Condition": {"StringEquals": {"ses:FromAddress": "no-reply@mail.example.com"}}
     }]
   }
   ```

   The statement has no `ses:SendRawEmail`, no `ses:*`, no read or admin action. No access key exists anywhere: the SDK uses the task role.
10. **Environment for the task definition (T-023)**: `SMEM_MAIL_PROVIDER=ses`, `SMEM_MAIL_FROM=SMemories <no-reply@mail.example.com>`, `SMEM_SES_CONFIGURATION_SET=smem-prod`, and `AWS_REGION`
    (Fargate sets it). None is a secret. The API refuses to start in prod without the first two.

### Check deliverability before launch

- DNS, from any shell: `dig +short CNAME <token>._domainkey.mail.example.com`, `dig +short MX bounce.mail.example.com`, `dig +short TXT bounce.mail.example.com`, `dig +short TXT _dmarc.mail.example.com`.
- Send a verification mail from the staging stack to a Gmail address and a Microsoft (Outlook.com) address: open "Show original" (Gmail) or the message source and confirm `SPF: PASS`, `DKIM: PASS` with
  `d=mail.example.com`, `DMARC: PASS`; the mail must land in the inbox, not in spam, and the Vietnamese subject and body must display correctly.
- Send one to a [mail-tester.com](https://www.mail-tester.com) address and aim for 9/10 or better.
- Register `mail.example.com` in Google Postmaster Tools. Read the first DMARC reports before leaving `p=none`.
- Run the mailbox-simulator addresses of step 8 once and read the result in the SES console (Reputation metrics, Suppression list).

## Local development

`SMEM_MAIL_PROVIDER` defaults to `log`: the message, code included, is printed to the API's stdout (the HTML part too, so you can paste it into a file to preview it). It refuses to exist outside
`SMEM_ENV=dev` or `test`. `SMEM_DEV_FIXED_OTP` and the other dev shortcuts are unchanged ([dev-shortcuts.md](dev-shortcuts.md)).

## Adding another provider

A provider is one file and one `case`. Checklist:

1. `internal/mailer/<provider>.go`: a type implementing `mailer.Mailer`; bounded timeouts and attempts; credentials from the platform's chain, not from a variable when the platform has one.
   Classify failures with `classify(ErrUnavailable|ErrRejected, <short code>, cause)`, or `classifyAs(ErrUnavailable, ClassConfig, <short code>, cause)` for a problem only the operator can fix (never put the provider's message, which may contain the address, in the reason). Keep every provider
   import and error name inside this file.
2. One `case` in `mailer.New` (`internal/mailer/mailer.go`) and one constant next to `ProviderSES`. Nothing else reads the provider name.
3. Its own variables under `SMEM_<PROVIDER>_*` (only `SMEM_MAIL_PROVIDER` and `SMEM_MAIL_FROM` are common), read in `internal/config`, with a prod rule if one is needed.
4. Rows for them in [production-config.md](production-config.md) (`TestEveryVariableIsDocumented` fails otherwise) and a refusal case in `internal/config/prod_test.go`.
5. A stub of the provider's API and a test that calls `mailertest.Run(t, newFixture)` (see `TestSESConformance` in `internal/mailer/ses_test.go`): text and HTML parts delivered, a cancelled context,
   throttling and 5xx -> `ErrUnavailable`, a permanent refusal of the address -> `ErrRejected`, an account or setup problem -> `ErrUnavailable` with `ClassConfig`, a hanging endpoint cut by the deadline, no recipient or code in logs or errors.
6. A section in this file for its owner-assisted setup, and the IAM or key handling.
