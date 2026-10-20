# Production configuration contract

Every `SMEM_*` variable the API (`smemories-api`), the migration task (`smemories-migrate`) and the backfill task (`smemories-media-backfill`) read, and what
`SMEM_ENV=prod` demands of them (T-022, T-086). The loader is [`internal/config`](../internal/config); `TestEveryVariableIsDocumented` fails when it reads
a variable that has no row below, and `TestProdContractRefusals` (table-driven) starts it with each missing or unsafe value and checks that the refusal
names the variable and echoes no secret. Image and run flags: [container.md](container.md).

**Email in prod.** The log mailer refuses `SMEM_ENV=prod` on purpose (`mailer.NewLog`); production sends through Amazon SES (T-021, [email.md](email.md)), so
`SMEM_MAIL_PROVIDER=ses` and `SMEM_MAIL_FROM` are required. The image smoke test runs the full API with `SMEM_ENV=test` and checks the prod refusals and the
one-off tasks with `SMEM_ENV=prod`.

**`dev` and `test` are local-only (T-099).** Both accept the public OTP key and the fixed code, so a deployed environment must be `prod`. In `dev` or `test` the loader (api,
migrate and backfill alike) refuses an `SMEM_ALLOWED_ORIGINS` entry or an `SMEM_PUBLIC_BASE_URL` whose host is not `localhost`, `*.localhost` or a loopback, private or link-local IP; the
message names the variable. LAN addresses stay allowed for phone testing at home; a public tunnel is refused on purpose. The image smoke test proves the missing-`SMEM_ENV` and public-base-URL refusals.

## Which command reads what

- **api**: everything.
- **migrate** (`up`, `status`, `version`, `down --allow-down`; deploy order, lock and exit codes in [deploy.md](deploy.md)): the database variables only. **backfill**: the database and object-store variables. Both load with `config.LoadMigrate`, which
  skips the API-only requirements (Redis, email codes, origins, base URL, memory limit) but still validates any variable that is set, so do not put API-only variables
  in their task definitions. The prod TLS rules for the database, the bucket rule and the S3 key pair apply to them as well.

## The table

Columns: required in prod; default; secret (never log, never put in plain env: Secrets Manager); where it comes from in AWS (`env` = plain environment variable in the
task definition, `SM` = Secrets Manager injected as an environment variable, `SSM` = Parameter Store); the commands that use it. Prod refusals are in the **Prod rule** column.

### Core

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_ENV` | yes, must be `prod` | none: required in every environment, an unset or empty value is refused (`SMEM_ENV is required: dev, test or prod`) | no | env | api, migrate, backfill | one of `dev`, `test`, `prod`. Anything but `dev`/`test` also forbids every dev shortcut. `dev` and `test` are local-only (see below). Set it in every task definition; the image sets none. |
| `SMEM_HTTP_ADDR` | no | `:8080` | no | env | api | `host:port`. |
| `SMEM_LOG_LEVEL` | no | `info` | no | env | api, migrate, backfill | `debug`, `info`, `warn`, `error`. |
| `SMEM_HTTP_REQUEST_TIMEOUT` | no | `30s` | no | env | api | at least `1s`. |
| `SMEM_TRUST_PROXY` | yes behind the ALB (`true`) | `false` | no | env | api | See [Client IP](#client-ip-and-smem_trust_proxy). |
| `SMEM_MEMORY_LIMIT_MIB` | yes | `0` (unset) | no | env | api | 64 to 1048576; about **80% of the task memory** (2 GiB minimum, so 1638). Image processing is memory hungry ([media.md](media.md)). |
| `SMEM_SHUTDOWN_TIMEOUT` | no | `25s` | no | env | api | 1s to 120s; keep below the 30 s Fargate stop timeout ([container.md](container.md)). |
| `SMEM_SHUTDOWN_DELAY` | no | `5s` in prod, `0s` elsewhere | no | env | api | at least 0 and below `SMEM_SHUTDOWN_TIMEOUT`. |
| `SMEM_UPLOAD_TMP_DIR` | yes in the image (a writable mount, e.g. `/spool`) | OS temp dir | no | env | api | must be an existing directory; the API stops at start when it is not writable. |
| `SMEM_ALLOWED_ORIGINS` | yes | none in prod (`http://localhost:5173` elsewhere) | no | env | api | comma-separated origins, every one **https**, no `user:pass@` (refused in every environment, never echoed). |
| `SMEM_PUBLIC_BASE_URL` | yes | none in prod (`http://localhost:5173` elsewhere) | no | env | api | **https** URL of the web app (links in emails, OAuth redirect); no `user:pass@` (refused in every environment, never echoed). |

### Database (MySQL / RDS)

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_DB_DSN` | yes | none | **yes** (carries the password) | SM | api, migrate, backfill | `user:pass@tcp(host:3306)/db`; must require **verified TLS**: `?tls=true` (system roots), or `SMEM_DB_TLS_CA_FILE`, or the opt-out below. `tls=false`, `preferred` and `skip-verify` are refused. |
| `SMEM_DB_TLS_CA_FILE` | no | none | no | env (path) | api, migrate, backfill | PEM bundle the server certificate is verified against (the RDS CA bundle, which the distroless system roots do not contain). Replaces `tls=` in the DSN. |
| `SMEM_DB_ALLOW_INSECURE` | no, **never on AWS** | `false` | no | not set | api, migrate, backfill | `true` lets prod use a DSN without TLS. For the image smoke test on a private docker network only. All three binaries (api, migrate, backfill) log `production safety opt-out is on` at start. |
| `SMEM_DB_MAX_OPEN` | no | `20` | no | env | api, migrate, backfill | integer >= 1. |
| `SMEM_DB_MAX_IDLE` | no | `SMEM_DB_MAX_OPEN` | no | env | api, migrate, backfill | integer >= 0. |
| `SMEM_DB_CONN_MAX_LIFETIME` | no | `5m` | no | env | api, migrate, backfill | non-negative duration. |

### Redis / Valkey (ElastiCache)

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_REDIS_URL` | yes | none | **yes** when it carries the token | SM | api | must be **`rediss://`** (TLS 1.2+) **with credentials** (`rediss://:token@host:6379/0`) unless the opt-out below. |
| `SMEM_REDIS_PASSWORD` | one of the two | empty | **yes** | SM | api | used when the URL carries no password; satisfies the credentials rule. |
| `SMEM_REDIS_ALLOW_INSECURE` | no, **never on AWS** | `false` | no | not set | api | `true` lets prod use `redis://` without credentials (image smoke test only). Logged at start. |
| `SMEM_REDIS_DIAL_TIMEOUT` | no | `2s` | no | env | api | positive duration. |
| `SMEM_REDIS_READ_TIMEOUT` | no | `1s` | no | env | api | positive duration. |
| `SMEM_REDIS_WRITE_TIMEOUT` | no | `1s` | no | env | api | positive duration. |
| `SMEM_REDIS_POOL_SIZE` | no | `10` | no | env | api | integer >= 1. |

### Object store (S3)

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_S3_BUCKET` | yes | `smemories-dev` outside prod | no | env | api, backfill | must be set. |
| `SMEM_S3_REGION` | no (the task has `AWS_REGION`; set it anyway) | `us-east-1` | no | env | api, backfill | |
| `SMEM_S3_ENDPOINT` | no | empty (AWS S3) | no | not set | api, backfill | for MinIO only; URL without credentials. In prod it must be `https://` unless `SMEM_S3_ALLOW_INSECURE` is set; leave it unset for AWS S3. |
| `SMEM_S3_ALLOW_INSECURE` | no, **never on AWS** | `false` | no | not set | api, backfill | `true` lets prod use an `http://` `SMEM_S3_ENDPOINT` (image smoke test on a private docker network only). Logged at start as `production safety opt-out is on`. |
| `SMEM_S3_PATH_STYLE` | no | `false` | no | not set | api, backfill | `true` for MinIO only. |
| `SMEM_S3_ACCESS_KEY` | **no: leave it unset** | empty | **yes** | not set | api, backfill | Empty means the [default credential chain](#s3-credentials-and-timeouts) (the ECS task role). Set it only together with `SMEM_S3_SECRET_KEY`. Static keys are **allowed** in prod but override the task role, so every binary logs a WARN naming `SMEM_S3_ACCESS_KEY` at start (decision: warn, not refuse, so the smoke test and a break-glass run still work). |
| `SMEM_S3_SECRET_KEY` | **no: leave it unset** | empty | **yes** | not set | api, backfill | Same: both or neither, otherwise the loader refuses. |

### Email (Amazon SES)

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_MAIL_PROVIDER` | yes, must be `ses` | `log` (prints the codes; dev and test only) | no | env | api | `log` or `ses`; prod refuses everything but `ses`. Runbook and provider checklist: [email.md](email.md). |
| `SMEM_MAIL_FROM` | yes | none | no | env | api | `SMemories <no-reply@mail.example.com>` or a bare address, on the verified sending subdomain; prod refuses a malformed address and an `example.*`, `.test`, `.invalid`, `localhost` domain. Needed in every environment when the provider is `ses`. |
| `SMEM_SES_CONFIGURATION_SET` | no (recommended) | empty | no | env | api | name of the SES configuration set that carries the reputation metrics. Region and credentials come from the AWS default chain (`AWS_REGION`, the ECS task role): no key variable exists. |

### Secrets and sign-in

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_OTP_KEY` | yes | none (fixed dev key in dev and test) | **yes** | SM | api | at least **32 bytes** (`openssl rand -base64 48`). Also derives the page-token key: rotating it only restarts clients' lists and invalidates outstanding email codes. |
| `SMEM_GOOGLE_CLIENT_ID` | no | empty = Google sign-in off | no | env | api | when set, the three below are required. |
| `SMEM_GOOGLE_CLIENT_SECRET` | with the client id | none | **yes** | SM | api | |
| `SMEM_OIDC_COOKIE_KEY` | with the client id | none | **yes** | SM | api | at least **32 bytes**. |
| `SMEM_GOOGLE_ISSUER` | no | `https://accounts.google.com` | no | env | api | **https** in prod (tests point it at a fake over http). |
| `SMEM_DEV_FIXED_OTP` | **must be unset** | unset | would be a backdoor | never | api | The loader refuses the API start when it is set and `SMEM_ENV` is not `dev` or `test`. Registered in [dev-shortcuts.md](dev-shortcuts.md); T-050 deletes it before the first deploy. |

### Limits and tuning

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_AUTH_MAX_CONCURRENT_HASHES` | no | `4` | no | env | api | integer >= 1. |
| `SMEM_AUTH_ARGON_MEMORY_KIB` | no | `19456` | no | env | api | 8 to 1048576. |
| `SMEM_AUTH_ARGON_TIME` | no | `2` | no | env | api | 1 to 100. |
| `SMEM_AUTH_ARGON_PARALLELISM` | no | `1` | no | env | api | 1 to 255. |
| `SMEM_RATE_REGISTER_PER_HOUR` | no | `5` | no | env | api | per client IP. |
| `SMEM_RATE_LOGIN_FAILS_PER_EMAIL` | no | `10` | no | env | api | per IP and email, 15 min. |
| `SMEM_RATE_LOGIN_FAILS_PER_IP` | no | `100` | no | env | api | 15 min. |
| `SMEM_MEDIA_MAX_BYTES` | no | `10485760` | no | env | api | 1 to 104857600. |
| `SMEM_MEDIA_MAX_CONCURRENT` | no | `2` | no | env | api | images decoded at once ([media.md](media.md)). |
| `SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP` | no | `8` | no | env | api | |
| `SMEM_PUBLIC_UPLOAD_MAX_CONNS` | no | `48` | no | env | api | |

### Metrics

| Variable | Required in prod | Default | Secret | AWS source | Commands | Prod rule |
|---|---|---|---|---|---|---|
| `SMEM_METRICS_ADDR` | no | empty = off | no | env | api | The listener has **no authentication**. In prod it must be a loopback or private IP (or `localhost`): `0.0.0.0`, `:port`, `[::]`, a public IP and any other host name are refused unless the opt-out below. Scrape it from inside the VPC; never route it through the ALB or CloudFront ([observability.md](observability.md)). |
| `SMEM_METRICS_ALLOW_ANY_INTERFACE` | no | `false` | no | env | api | `true` allows binding every interface, for a private task network where the security group is the control (the scraper cannot reach loopback). Logged at start. |

## Client IP and `SMEM_TRUST_PROXY`

`SMEM_TRUST_PROXY=true` takes the client address from the last `X-Forwarded-For` hop. Set it **only** when the task is reachable solely through the ALB (security group allows
the ALB only): then that hop is the one the ALB appended and the per-IP rate limits see real clients. Without it every request shares the ALB's address and the limits lock everybody
out together; with it on a task that clients can reach directly, any client can forge its address and walk around the limits.

## S3 credentials and timeouts

With `SMEM_S3_ACCESS_KEY` empty the client uses the **AWS default credential chain**: environment variables, web identity, shared config, the **ECS task role** (the container credentials
endpoint Fargate provides) and the EC2 instance role, refreshed by the SDK before they expire. Give the task role `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket`
on the bucket and nothing else; no static key exists anywhere. Static keys (both variables) still work, for MinIO in dev and test. A task without a role starts, and its first storage call
fails with a credentials error (`/readyz` does not check the bucket).

The client no longer waits the AWS default (about 25 s on a refused connection, repeated by silent retries). Fixed values, in `storage.S3Config`: dial 3 s, TLS handshake 5 s, response
headers 15 s, one operation end to end (retries and body included) 60 s, at most 3 attempts. They are constants, not variables: nothing in production needs to change them.
