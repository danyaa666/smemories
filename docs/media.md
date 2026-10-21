# Photo processing: memory budget

## Routes (T-069, v2 contract)

| Route | Notes |
|---|---|
| `POST /api/media/create` | `multipart/form-data`: text field `yearbook_id` and the image in the part `file` (either order). Answers `{status:"OK", data:{id, yearbook_id, content_type, bytes, width, height, created_at}}`. |
| `GET /api/media/get-list?yearbook_id=&limit=&next_id=&uploader=` | `data: {items, next_id}`, newest first, `limit` 1..100 (default 50). `uploader` is `owner` (default), `contributor` or `all`. |
| `GET /api/media/get-content?id=&variant=display\|thumb\|print` | **Binary** on success (no envelope; `Content-Type` from the database, `nosniff`, `Cache-Control: private, max-age=3600`, `ETag` per variant, `HEAD`/`Range`/`If-None-Match` work); the JSON error envelope with `Cache-Control: no-store` on failure. |
| `POST /api/media/delete` | JSON `{id}`; idempotent. |
| `GET /api/public-media/get-content?token=` | **Public** (no session, no cookie, no CORS), **binary** display variant only, behind a signed link token; see "Public photo links" below. |

Errors are the v2 envelope (`ERROR_INVALID_IMAGE` 400, `ERROR_UNSUPPORTED_MEDIA_TYPE` 415, `ERROR_QUOTA_EXCEEDED` 409, `ERROR_TOO_LARGE` 413, `ERROR_RATE_LIMITED` 429, `ERROR_BUSY` 503, `ERROR_STORAGE_ERROR` 502, `ERROR_NOT_FOUND` 404, `ERROR_PARAM` 400).

- **Deadlines.** The server's own timeouts (15 s read, 30 s write, `httpx.NewServer`) are too short for a phone upload or a large download, so `create` and `get-content`
  carry a per-route timeout (`httpx.WithTimeout`: 2.5 min for create = 2 min to receive the body + 30 s to process, 2 min for get-content) and, in the same wrapper (`slowRoute`),
  move the connection's write deadline to the route timeout plus 10 s (and the read deadline of create to 2 min). A route timeout longer than `WriteTimeout` without that
  would be cut off by the server; `TestSlowRouteSurvivesServerWriteTimeout` proves it against a real server.
- **Page tokens.** `next_id` is `httpx.Cursors.Seal(scope, position)`: AES-256-GCM of the position with the scope (`media/get-list:<yearbook id>`) as associated data, base64url. A
  client cannot read the internal id inside or change it, and a token from another yearbook or another list is `ERROR_PARAM`. The key is derived (HMAC-SHA256 with a fixed label) from
  `SMEM_OTP_KEY`, so there is no extra secret to configure; changing that key only makes clients start their list again. The same helper serves the other list endpoints.

Every upload is decoded, scaled to at most 3000 px (print 1800 px, thumbnail 480 px) and re-encoded by `process` in
`internal/media/image.go`. Decoding is the memory risk: a few hundred KB of PNG can claim a 49 megapixel
canvas. The limits below make the worst case a number we chose.

## Limits (all checked from the header, before any pixel buffer exists; failure is `400 invalid_image`)

| Limit | Value | Why |
|---|---|---|
| Side | 12000 px | outer bound |
| Pixels, JPEG | 50 MP | a baseline 4:2:0 JPEG decodes to 1.5 bytes per pixel |
| Pixels, PNG and WebP | 25 MP | they decode to 4 to 8 bytes per pixel |
| Estimated decoded size | 128 MiB | see below; this is the cap that bounds memory |
| `SMEM_MEDIA_MAX_CONCURRENT` | 2 | images decoded at once; the others wait holding only their upload (at most 10 MiB) |
| `SMEM_MEMORY_LIMIT_MIB` | 0 (unset) | soft Go memory limit (`debug.SetMemoryLimit`); in a container use about 75% of the task memory |

Estimated decoded size is width x height x bytes per pixel: 8 for 16-bit RGBA, 4 for 8-bit RGBA, 2 for 16-bit gray, 1 for 8-bit gray or
palette, 2.5 for lossy WebP. A JPEG's colour model is always YCbCr, so the subsampling is read from its SOF segment (1.5 for 4:2:0, 3 for 4:4:4),
and a progressive JPEG counts five times that, because the Go decoder keeps 4 bytes of coefficients per sample until the last scan. A JPEG
whose SOF cannot be read counts as progressive 4:4:4. Largest accepted: 4096 x 4096 16-bit PNG, 5000 x 5000 8-bit PNG or WebP,
10000 x 5000 baseline 4:2:0 JPEG, 6000 x 7456 baseline 4:4:4 JPEG, about 18 MP progressive 4:2:0 JPEG.

Photos sent through a collection link (`POST /api/public-collect/submit-note`, `media.Service.UploadContributorFrom`, T-034, v2 since T-068) use the same processing slots and limits, but the upload
does not hold one. The whole body (at most 32 MiB) is first **spooled to a temporary file** while the phone sends it, so a slow client costs a
socket and some disk, never memory and never a processing slot. Only when the body is complete are the text validated and the photos
processed, one at a time: each photo is read from the file into memory **after** a processing slot was obtained (at most
`SMEM_MEDIA_MAX_BYTES` per slot) and the request waits at most 10 s for a slot, otherwise `503 ERROR_BUSY`.

| Setting | Default | Meaning |
|---|---|---|
| `SMEM_PUBLIC_UPLOAD_MAX_CONNS` | 48 | public note submissions in progress at once (all addresses); the next one is `503 ERROR_BUSY` |
| `SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP` | 8 | ... of which one client IP may have this many |
| `SMEM_UPLOAD_TMP_DIR` | OS temp dir | where the bodies are spooled (files `0600`, `smem-upload-*`, removed when the request ends, also on error, panic and disconnect) |

A refused submission (`503 ERROR_BUSY`, 2 to 6 s random `Retry-After`) is answered before any byte of its body is read and is not counted against
the per-IP request limits, so a class that retries cannot lock itself out. A body must keep moving: no byte for 10 s, or less than 16 KiB/s on
average after a 15 s grace period, or more than 120 s in total is dropped with `408`. The link is resolved first, from the query parameter `token`
(T-034 order, kept by T-068): an unknown, malformed or revoked token is a `404` sent at once, without reading or spooling the body. The other rejections (full link, closed link,
rate limit, bad text) are sent only after the rest of the body has been read (same limits), so a browser shows the answer instead of a connection reset.

**Disk sizing.** Worst case is `SMEM_PUBLIC_UPLOAD_MAX_CONNS` x 32 MiB = 1.5 GiB of temporary files (48 x 32 MiB), in practice far less. In production
(T-022) the ephemeral storage of the task, or the volume behind `SMEM_UPLOAD_TMP_DIR`, must exceed that plus headroom. A process that is killed (or whose drain
is cut off) leaves its files behind: at start-up, before the listener accepts traffic, the API deletes every regular file named `smem-upload-*`
directly in the spool directory (`SMEM_UPLOAD_TMP_DIR`, else the OS temp dir) that is older than 4 min (the 3 min route timeout plus 1 minute), so a live
upload is never touched; sub-directories and symlinks are left alone, a failure is only a WARN, and one INFO line gives the count removed (T-074).
Ephemeral storage is cleared on restart anyway. Many-address floods are the job of the WAF and load balancer limits (T-031).

## Public photo links (T-092, decision D-32)

The plain-text export of a book gives every photo as a link that opens **without a session**. Photos are third parties' personal data, so this is the one anonymous way to
fetch a photo and it carries the controls the owner accepted with D-32. `internal/medialink` (pure functions), `media.Service.PublicContent`, `GET /api/public-media/get-content?token=`
and `POST /api/yearbook/revoke-photo-links`.

- **Token** = `<photo ULID>.<base64url(first 16 bytes of HMAC-SHA256(SMEM_MEDIA_LINK_SECRET, "pm1|" + media id + "|" + yearbook id + "|" + link_epoch))>`, 49 characters. The comparison is
  constant time (`hmac.Equal`) and a MAC has exactly one valid spelling (strict base64url). The secret is `SMEM_MEDIA_LINK_SECRET` ([production-config.md](production-config.md)); rotating it ends every link.
- **Revoke.** `yearbook_tab.link_epoch` (migration `20261019100000`) is part of the MAC. `POST /api/yearbook/revoke-photo-links {id}` adds 1 atomically: every earlier link of that book fails at once.
  There is no per-photo revoke and no expiry (D-32). 10 calls per hour per user (`revoke_links`, fails open).
- **What a link opens.** The photo is the cover or a profile photo of its book, or belongs to an `approved` note of that book (`media.ShareableNoteStatus`). Hiding or deleting the note, or deleting the photo or the book, stops the link at once.
  The MAC and that test are evaluated in one query, and **every** failure (malformed token, unknown photo, wrong MAC, old epoch, pending/hidden/deleted note, photo no one refers to, deleted book or photo, a missing stored object)
  is the same `404 ERROR_NOT_FOUND` with the same body (tests compare the bytes with a pinned `X-Request-Id`), so the endpoint is not an oracle.
- **What is served.** The `display` variant only (the re-encoded image: no EXIF, GPS, XMP, ICC or comment segment survives, `TestPublicPhotoCarriesNoMetadata`; never the original, there is none stored, and never `thumb` or `print`).
  Headers: `Content-Type` from the database, `X-Content-Type-Options: nosniff`, `Content-Disposition: inline`, `X-Robots-Tag: noindex, nofollow`, `Referrer-Policy: no-referrer`, `Cross-Origin-Resource-Policy: cross-origin`,
  `Cache-Control: public, max-age=300`. `HEAD`, `Range` and `ETag`/`If-None-Match` work as in `media/get-content` (same `writeContent`); a refused link is never 304.
- **A revoked link can still be served from a cache for up to 5 minutes** (`max-age=300`, and `public` lets CloudFront or any shared cache keep it). Same for a photo whose note was just hidden. This is part of what the owner accepted in D-32; shortening
  it means changing `publicCacheControl`.
- **Limits** per client IP, both fail open ([redis.md](redis.md)): `public_media_miss` 60 per 15 minutes (only misses count; a good link gives its hit back), `public_media_all` 600 per 15 minutes. The route timeout (2 minutes) and the connection deadline
  extension of `get-content` apply; the bytes are read from the object store into memory like `get-content` (no processing slot is used: nothing is decoded).
- **The token is a bearer credential and is never logged.** The access log records the route (`GET /api/public-media/get-content`), never the query; no error, log line, metric label or `/metrics` text contains a token or the secret
  (`TestPublicTokenNeverReachesLogsOrMetrics` runs a request with a recognisable token down every path, storage failure and 429 included, and greps the captured output). Metric: `smem_public_media_requests_total{result="ok|not_found|rate_limited"}`.
- **For T-023 / T-024 (edge and logging).** CloudFront and ALB **access logs record the query string**, so those logs hold live bearer tokens for `/api/public-media/get-content` (and for `/api/public-collect/*`):
  give the log buckets the same access control and retention care as the logs of `public-collect`, and do not copy them into a general-purpose search. The `public` cache header lets the edge cache by full URL.

## Print size (T-057, decision D-24)

Three objects per photo: `<id>.<ext>` (display, 3000 px), `<id>-print.<ext>` (print, long edge 1800 px, JPEG quality 85, PNG with
transparency stays PNG) and `<id>-thumb.jpg` (480 px). `media.print_key` holds the print key (migration 0013, NULL for photos older than
the migration). `GET /api/media/get-content?id=<id>&variant=print` serves it with the headers and authorisation of the other sizes (ETag ends in `-p`);
while `print_key` is NULL it answers in display size, so the print renderer works before and during the backfill.

- A photo whose long edge is at most 1800 px keeps its display bytes as the print object (no second lossy pass).
- The thumbnail is scaled from the print size (cheaper than from 3000 px); the print version is scaled and encoded in a goroutine
  while the display is encoded. Process time of a 4032 x 3024 JPEG, median of 9 on a laptop: 0.90 s before, 0.86 s after. On a single
  core the extra work is about 20%. Peak memory with the two largest accepted inputs (49 MP JPEG, 25 MP PNG), 2 at a time: 954 MiB
  before, 960 MiB after (the print buffer is 1800 x 1350 x 4 bytes, about 10 MB).
- Deleting a photo, discarding a failed submission or deleting a yearbook removes the print object too; a failed print write leaves nothing behind.

**Backfill.** `bin/smemories-media-backfill [--dry-run] [--batch 100] [--max-consecutive-failures 20]` creates the print object
of every photo with `print_key IS NULL` from its stored display object, oldest first. It is idempotent (finished photos are never selected; a crash only
repeats one write), can be stopped with Ctrl-C and restarted, and holds one image in memory at a time. It prints
`print objects created: N, skipped: N, failed: N` (skipped: deleted meanwhile; failed: display object missing or unreadable, logged by media id, retried by
the next run) and exits 1 when something failed. Run it once after `make migrate` / the deploy that carries migration 0013; locally `make build && bin/smemories-media-backfill`.

- **Environment (T-075).** It loads its configuration like `smemories-migrate` (`config.LoadMigrate`): `SMEM_DB_DSN` plus the object-store settings
  (`SMEM_S3_BUCKET`, `SMEM_S3_REGION`, `SMEM_S3_ENDPOINT` and `SMEM_S3_PATH_STYLE` for MinIO, `SMEM_S3_ACCESS_KEY` / `SMEM_S3_SECRET_KEY`) and the optional `SMEM_ENV`, `SMEM_LOG_LEVEL`, `SMEM_DB_MAX_*`. It does **not** need `SMEM_REDIS_URL`, `SMEM_OTP_KEY`, `SMEM_ALLOWED_ORIGINS`
  or `SMEM_PUBLIC_BASE_URL`; values that are set are still validated. The one-off task definition (T-022) carries exactly this list.
- **Object-store outage.** After `--max-consecutive-failures` (default 20) object-store errors in a row (a read that is not "not found", or a write) the run stops with
  exit 1 and "object store keeps failing", instead of walking every photo at about 2.5 s each. A photo that fails on its own (display object missing or corrupt) does not count and
  resets the streak.
- **Dry run.** `--dry-run` reads and decodes every display object like a real run (so it takes about as long minus the writes) and writes nothing; "would create" is therefore exact,
  and corrupt or missing display objects show as `failed`.
- **Delete race.** If a photo is deleted between the print write and the row update, the backfill deletes the object it just wrote (it checks that the row is gone; a parallel
  backfill that filled the row keeps the object, which has the same key).
- **No gain.** Like photos not larger than 1800 px, a photo whose 1800 px encoding is not smaller than its display bytes (only seen with synthetic PNGs) gets the display bytes as its print object.

## What was wrong, and the fix

`golang.org/x/image/draw` allocates a `float64` scratch buffer of (destination width x SOURCE height x 4) in one scaling call: 480 MB for a
50 MP photo, several times the picture. `resize` now scales the width in bands of 128 source rows, then the height in bands of 128 columns
(each pass leaves the other axis at scale 1, an exact identity for CatmullRom), so the scratch stays at 12 to 30 MB and the picture is the same
(`TestResizeBandsMatchOneShot`). The alpha check runs before the scale, so the decoded image is not referenced afterwards and can be collected.

## Measurement

Peak resident set size (macOS, Go 1.26, `/usr/bin/time -l`) of the whole test process, one request at a time. "Before" is the code of T-009
at the worst input it accepted (7000 x 7000 or 10000 x 5000), "after" is the largest input still accepted.

| Input | Before | After |
|---|---|---|
| PNG, 16-bit RGBA, 49 MP / 4096 x 4096 | 1166 MB | 490 MB |
| PNG, 8-bit RGBA, 49 MP / 5000 x 5000 | 971 MB | 445 MB |
| WebP lossless, 49 MP / 25 MP | 1004 MB | 444 MB |
| WebP lossy with alpha, 25 MP | 883 MB (49 MP) | 371 MB |
| JPEG baseline 4:2:0, 50 MP | 642 MB | 391 MB |
| JPEG baseline 4:4:4, 50 MP / 35 MP | 706 MB | 444 MB |
| JPEG progressive 4:4:4, 50 MP / 8.7 MP | 1296 MB | 276 MB |

Four uploads at once with the default concurrency of 2: 4 x 16-bit PNG 4096 x 4096 peak at 970 MB (925 MiB), a mix of the four largest
formats peaks at 918 MB, both under the 1.2 GiB target (a 2 GiB task keeps headroom). The 4-parallel run with a concurrency of 4, as it was
before, peaked at 3.2 GB. A third of the peak is garbage waiting for the collector (the default `GOGC` lets the heap grow to twice the live
size); `SMEM_MEMORY_LIMIT_MIB` makes the collector run earlier near the limit.

Reproduce (the inputs are not in the repository; make them with any image tool):

```
SMEM_MEASURE_FILES=a.png,b.jpg SMEM_MEASURE_PAR=2 SMEM_MEASURE_CONC=2 /usr/bin/time -l go test ./internal/media -run MeasurePeak -v
```

On Linux read "Maximum resident set size" from `/usr/bin/time -v` instead (in KiB).
Re-measure after changing a limit, the scaling code or the Go version, and update the tables.
