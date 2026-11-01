# Photo processing: memory budget

## Routes (T-069, v2 contract)

| Route | Notes |
|---|---|
| `POST /api/media/create` | `multipart/form-data`: text field `yearbook_id`, the image in the part `file` and the optional text field `purpose` (`design`; see "Design images"), in any order. Answers `{status:"OK", data:{id, yearbook_id, content_type, bytes, width, height, created_at, purpose}}`. |
| `GET /api/media/get-list?yearbook_id=&limit=&next_id=&uploader=&purpose=` | `data: {items, next_id}`, newest first, `limit` 1..100 (default 50). `uploader` is `owner` (default), `contributor` or `all`. Photos only, unless `purpose=design`. |
| `GET /api/media/get-content?id=&variant=display\|thumb\|print` | **Binary** on success (no envelope; `Content-Type` from the database, `nosniff`, `Cache-Control: private, max-age=3600`, `ETag` per variant, `HEAD`/`Range`/`If-None-Match` work; readable by the yearbook's owner and, read only, by the admin of a class whose member linked the yearbook, for its profile photo, the photos of approved notes and its design images only (docs/class-permissions.md); streamed from the object store, see "Photo downloads"); the JSON error envelope with `Cache-Control: no-store` on failure. |
| `POST /api/media/delete` | JSON `{id}`; idempotent. |
| `GET /api/public-media/get-content?token=` | **Public** (no session, no cookie, no CORS), **binary** display variant only, behind a signed link token; see "Public photo links" below. |

Errors are the v2 envelope (`ERROR_INVALID_IMAGE` 400, `ERROR_UNSUPPORTED_MEDIA_TYPE` 415, `ERROR_QUOTA_EXCEEDED` 409, `ERROR_DESIGN_IMAGE_LIMIT` 409, `ERROR_TOO_LARGE` 413, `ERROR_EMAIL_NOT_VERIFIED` 403 (create), `ERROR_RATE_LIMITED` 429, `ERROR_BUSY` 503, `ERROR_STORAGE_ERROR` 502, `ERROR_NOT_FOUND` 404, `ERROR_PARAM` 400).

- **Deadlines.** The server's own timeouts (15 s read, 30 s write, `httpx.NewServer`) are too short for a phone upload or a large download, so `create` and `get-content`
  carry a per-route timeout (`httpx.WithTimeout`: 2.5 min for create = 2 min to receive the body + 30 s to process, 2 min for get-content) and, in the same wrapper (`slowRoute`),
  move the connection's write deadline to the route timeout plus 10 s (and the read deadline of create to 2 min). A route timeout longer than `WriteTimeout` without that
  would be cut off by the server; `TestSlowRouteSurvivesServerWriteTimeout` proves it against a real server.
- **Upload fairness (T-102, T-110).** `create` needs a **verified email** (`403 ERROR_EMAIL_NOT_VERIFIED`, the rule of the moderation endpoints, `docs/moderation.md`): registration is open, so without it
  throwaway accounts could hold the pool. That check, and the three caps below in this order, are decided before any body byte is read: (1) the **per-user cap**, at most 2 uploads in progress per internal user id
  (`maxUploadsPerUser`); (2) the **per-client cap**, at most 4 uploads in progress per client key (`maxUploadsPerClient`, `httpx.ClientIP`: the address, the /64 for IPv6), so several accounts behind one address
  cannot fill the pool; both refusals are `429 ERROR_RATE_LIMITED` with `Retry-After: 5`; (3) the **shared pool** of 4 x `SMEM_MEDIA_MAX_CONCURRENT` slots (8 by default; it bounds the memory of request
  buffers, so it is taken before the body is read), a full pool is `503 ERROR_BUSY`. Then the body is read under `httpx.PhoneUploadPace`, the same pace as the public route: no byte for 10 s, or under 16 KiB/s
  on average after a 15 s grace, or more than 2 min in total ends the upload with `408` and gives every slot back (a 10 MiB photo over 2 Mbit/s takes about 40 s and passes). The per-user and per-client counts are
  in-process maps released by `defer` on every path; with N API tasks one user can hold 2 x N uploads and one address 4 x N, which is still bounded by each task's own pool. `smem_media_uploads_in_progress` shows the pool.
- **Photo downloads (T-107).** Both download routes **stream** the object from the store (`storage.Storage.Open`: a `HEAD` for the size, then a ranged `GET bytes=<position>-` on the first read), so a download holds
  one 32 KiB copy buffer, not the photo: 64 slow readers of an 8 MiB photo grow the heap by about 7 MiB, where whole-object buffering held 512 MiB (`TestSlowReadersDoNotHoldTheObjectInMemory` logs heap and RSS). `Content-Length`,
  `ETag`, `Range` (open, suffix and middle ranges: the store GET starts at the range start and is closed when the range is sent, so nothing before it is read; the connection of an early close is not reused), `If-None-Match`, `If-Range` and `HEAD` behave as before (`http.ServeContent` over a seekable stream; `HEAD` and 304 never fetch a body). Because the stream
  is bounded by size, no byte budget (`SMEM_MEDIA_DOWNLOAD_MAX_BYTES`) exists. Each response is under `httpx.PaceWriter`: every write moves the connection's write deadline 10 s ahead, and a reader that averages under
  16 KiB/s after a 15 s grace (or takes over 2 min) is dropped, which ends the stream and frees the slot, so a slow reader no longer holds a store connection for the whole route timeout. **Fairness:** at most 64 downloads in
  progress per account (owner route, internal user id) and per client address (public route, `httpx.ClientIP`), the next is `429 ERROR_RATE_LIMITED` with `Retry-After: 2`, answered before the limiters, the database and the store
  (and counted as `rate_limited` on the public route). The count is an in-process map (with N API tasks the cap is N x 64). **Why 64 and not 8 (QA of T-107):** over HTTP/2, which CloudFront and the ALB give browsers, a gallery page requests all its images at once
  (the browser's 6-per-host limit of HTTP/1.1 does not apply), and an `<img>` does not retry a 429, so a cap of 8 left 22 of 30 thumbnails broken. Streaming already keeps memory flat (a download is one 32 KiB buffer), so the cap
  no longer protects memory; it only bounds open connections and store requests per client. 64 lets a page of 30 thumbnails load completely. A page of 100 starts more than 64 requests in the same instant (Chromium over HTTP/2 through a proxy: 83 of 100 rendered, 17 answered 429, with the cap alone), so a client
  at the cap **waits up to `downloadGrace` (500 ms) for a slot** before it is refused: thumbnails end within milliseconds, so the 100 load completely, while a client that really has 64 slow downloads open is still refused (`429`, `Retry-After: 2`) after the wait.
  Tests: `TestGalleryPageOfThumbnailsLoadsAtOnce` (30 and 100 truly parallel requests), `TestDownloadAtTheCapWaitsForAFreedSlot`, the 65th-answers-429 tests of both routes, and `tools/e2e/h2-gallery.spec.mjs` (part of `make e2e`: real Chromium loads 30 and 100
  thumbnails through a TLS proxy over HTTP/2 and over HTTP/1.1, one HTTP/1.1 request per stream to the API as the edge does). The cap is the constant `maxDownloadsPerClient`, the wait `downloadGrace`. `smem_media_download_inflight` ([observability.md](observability.md)) shows the downloads in progress.
- **Page tokens.** `next_id` is `httpx.Cursors.Seal(scope, position)`: AES-256-GCM of the position with the scope (`media/get-list:<yearbook id>`) as associated data, base64url. A
  client cannot read the internal id inside or change it, and a token from another yearbook or another list is `ERROR_PARAM`. The key is derived (HMAC-SHA256 with a fixed label) from
  `SMEM_OTP_KEY`, so there is no extra secret to configure; changing that key only makes clients start their list again. The same helper serves the other list endpoints.

Every upload is decoded, scaled to at most 3000 px (print 1800 px, thumbnail 480 px) and re-encoded by `process` in
`internal/media/image.go`. Decoding is the memory risk: a few hundred KB of PNG can claim a 49 megapixel
canvas. The limits below make the worst case a number we chose.

## Design images (T-152, decision D-43)

A student may upload their own background and decoration images for the layout editor. They are `media_tab` rows with `purpose = 'design'` (migration `20261031120000`: `VARCHAR(16) NOT NULL DEFAULT 'photo'`, expand-only,
no index because every query is scoped by `yearbook_id`; its Down deletes the design rows, because the previous schema could not tell them from photos, and drops the column; the objects go with the yearbook's prefix).
They are decoded, stripped of metadata, re-encoded and limited exactly like photos (same pipeline, limits, rate limits, per-user and per-client caps and memory budget) and have the same three versions.

- **Which version a layout uses.** All three variants are stored; nothing new is needed. `display` (long edge at most 3000 px, not the 2000 px the task text assumed) is the version for a full-bleed background: an A4 page at 300 dpi needs
  2480 x 3508 px, so a 3000 px long edge is about 256 dpi on A4 (and over 300 dpi on A5), which is acceptable for a background. The `print` variant (1800 px) is **too small for a full-bleed A4 background**: the layout runtime must request
  `display` for backgrounds and may use `print` for small decorations. No new variant was added.
- **Isolation.** The default of `get-list` and every photo query is `purpose = 'photo'`; `purpose=design` lists only design images (they are always the owner's own, so `uploader=contributor` lists none). The page tokens of the two lists
  are not interchangeable (the purpose is part of the token's scope). A design id is not a cover or a profile photo (`yearbook` refuses it with `ERROR_INVALID_MEDIA`, the same code as an unknown photo), friends' note photos are always photos,
  and no public link opens a design image (`publicPhoto` requires `purpose = 'photo'`). The other direction, a photo as a design image, is `Service.DesignImages(ctx, owner, yearbookID, ids)`, which returns the ids that are design images of
  that owner's yearbook: the layout task must use it before it stores a media id (`TestDesignImageIsolation`).
- **Limits.** At most 30 design images per yearbook (`maxDesignPerYearbook`); they do not count toward the 200 photos. Their stored bytes (display + thumb + print) count with the photos' in the owner's 500 MiB per user. The 31st design image is `ERROR_DESIGN_IMAGE_LIMIT` (409, `data: {limit: 30}`, T-164); the 500 MiB quota stays `ERROR_QUOTA_EXCEEDED` (409) for photos and design images alike.
- **Delete.** `media/delete` works for a design image (idempotent, objects first). Deleting the yearbook deletes its design images with the rows and the prefix (`TestDesignImageDelete`). A layout that still references a deleted
  image is the reader's problem (`book-data/get` drops the background or item, T-153); deletion never checks for references.
- **Class admin.** `media.Store.readable` also opens a design image of a member's linked book for the class admin (read only, `no-store`) **only while the member's stored layout references it** (T-153: `bg.media` or an `image` item of `book_layout_tab.layout_json`; before T-153 every design image of the book was readable); see docs/class-permissions.md and docs/class-privacy-review.md.
- **Metrics.** `smem_media_uploads_total{purpose,result}` ([observability.md](observability.md)).
- **Export and account deletion (T-025).** There is no account deletion or export endpoint yet. When T-025 adds them: a design image is a row of its book, so the yearbook delete path covers it; the export lists a book's design images with
  `media/get-list?purpose=design` (id, size, created_at), the same way it lists photos.

## Quotas (T-105)

Both quotas count **every variant a photo stores** (display, thumb and print: `media_tab.bytes + thumb_bytes + print_bytes`; a photo uploaded before T-105 has 0 for the two new columns until the print backfill fills `print_bytes`).
`ERROR_QUOTA_EXCEEDED` (409) is the answer for either.

| Quota | Counts | Limit | Constants |
|---|---|---|---|
| Owner | photos the owner uploads (`uploader_kind = 'owner'`, `purpose = 'photo'`) | 200 photos per yearbook, 500 MiB per user across yearbooks (photos and design images together) | `maxPerYearbook`, `maxBytesPerUser` |
| Owner, design images (T-152) | design images the owner uploads (`purpose = 'design'`) | 30 per yearbook; the bytes count in the 500 MiB above | `maxDesignPerYearbook` |
| Contributor | photos friends send through a link (`uploader_kind = 'contributor'`) | 600 photos and 1 GiB per yearbook | `maxContributorPhotos`, `maxContributorBytes` |

The two never count toward each other: someone who holds a leaked link can fill the contributor quota with junk but cannot take away one photo of the owner's room (`TestContributorQuotaIsSeparate`), and a full owner quota does not stop
friends from sending photos. Over the contributor quota the public submit answers `409 ERROR_QUOTA_EXCEEDED` with the message "this yearbook cannot take more photos right now, send your note without photos" (`media.ErrContributorQuota`); the same
note without photos is accepted (`TestSubmitLimits`). Both are checked early (cheap, before decoding) and again in the insert transaction under the owner's user-row lock, so parallel uploads cannot slip past. The owner can see and delete
contributor photos in the gallery (`uploader=contributor`), and moderation delete frees the room.

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
palette, 2.5 for lossy WebP and 8.75 for lossy WebP with an ALPH chunk (T-113: `golang.org/x/image/webp` decodes the alpha through `vp8l`, which builds a
transient NRGBA of 4 bytes per pixel plus up to 2 bytes per pixel of packed palette indices, on top of the 1.5 of the frame and the 1 of the alpha plane;
measured 8.5, 203 MiB for 5000 x 5000, against the 2.5 that was assumed before). A JPEG's colour model is always YCbCr, so the subsampling is read from its SOF segment (1.5 for 4:2:0, 3 for 4:4:4),
and a progressive JPEG counts five times that, because the Go decoder keeps 4 bytes of coefficients per sample until the last scan. A JPEG
whose SOF cannot be read counts as progressive 4:4:4. Largest accepted: 4096 x 4096 16-bit PNG, 5000 x 5000 8-bit PNG or lossy or lossless WebP without alpha, about 3800 x 3800 lossy WebP with alpha,
10000 x 5000 baseline 4:2:0 JPEG, 6000 x 7456 baseline 4:4:4 JPEG, about 18 MP progressive 4:2:0 JPEG.

## Before and after decode, and panics (T-104)

- **Before decode** (`image.DecodeConfig`, the header only): the content type is sniffed, then the side, pixel and estimated-memory limits above are applied to the size the header reports.
- **After decode** (`matchesHeader`): the decoded image must have exactly the size the header reported, which is the size the limits judged; any difference is `400 ERROR_INVALID_IMAGE`. A lossy WebP with transparency (`NYCbCrA`)
  must also have an alpha plane that covers the whole picture, because the standard library slices it by the luma rectangle. `printOf` (backfill) does the same.
- **WebP VP8X canvas against frame.** `DecodeConfig` of a VP8X file returns the canvas size without reading the frame. `golang.org/x/image/webp` (v0.46.0, `decode.go`) compares the VP8 or VP8L frame header with the canvas
  *before* it allocates and answers `errInvalidFormat` on a mismatch, so a tiny canvas with a huge frame never gets a big buffer. `testdata/vp8x-canvas-mismatch.webp` (58 bytes: 1x1 canvas with alpha, 1x1 ALPH, VP8 frame header
  8000 x 8000) pins that: `TestProcessRefusesAWebPWhoseFrameIsLargerThanItsCanvas` checks the refusal and that it allocates next to nothing. Re-run it when `golang.org/x/image` is upgraded.
- **Slots.** `Service.processPhoto` takes the processing slot and gives it back with `defer`, so no path leaks one (the gauge `smem_media_processing_slots_in_use` returns to 0). Loading from the spool file happens inside it.
- **Panics.** Load, decode, scale and encode run under `recover`, and so does the goroutine that scales and encodes the print version (a panic in a goroutine is not caught by the HTTP recover middleware and would end the process).
  A recovered panic answers `400 ERROR_INVALID_IMAGE` (the client sees no detail), writes one ERROR line "image processing panicked" (`where`, `panic`, `stack`; never image bytes) and increases
  `smem_media_process_panics_total` ([observability.md](observability.md)). Any increase means a decoder bug or a crafted file: look at the stack.

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
rate limit, bad text) are sent only after the rest of the body has been read (same limits), so a browser shows the answer instead of a connection reset. A request refused before its photos are processed (rate limit, closed or full link) gives its upload slot back **before** it drains the rest of the body (T-102): while draining it holds only a socket, never one of the `SMEM_PUBLIC_UPLOAD_MAX_CONNS` slots. Those drains are bounded too (T-110): at most 16 rejected bodies per client key are drained at once (`drainsPerClient`, counted in `smem_public_drains_in_progress`); beyond that the server sends the error with `Connection: close` and does not read the rest, so a browser behind such an address may see a connection reset instead of the answer.

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
  extension of `get-content` apply; the bytes are streamed from the object store like `get-content`, with the same 64-per-address cap (no processing slot is used: nothing is decoded).
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
  (`SMEM_S3_BUCKET`, `SMEM_S3_REGION`, `SMEM_S3_ENDPOINT` and `SMEM_S3_PATH_STYLE` for a local store, `SMEM_S3_ACCESS_KEY` / `SMEM_S3_SECRET_KEY`) and the required `SMEM_ENV` (no default) and the optional `SMEM_LOG_LEVEL`, `SMEM_DB_MAX_*`. It does **not** need `SMEM_REDIS_URL`, `SMEM_OTP_KEY`, `SMEM_ALLOWED_ORIGINS`
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
| WebP lossy with alpha, 25 MP | 883 MB (49 MP) | 371 MB (T-104; see the T-113 note below) |
| JPEG baseline 4:2:0, 50 MP | 642 MB | 391 MB |
| JPEG baseline 4:4:4, 50 MP / 35 MP | 706 MB | 444 MB |
| JPEG progressive 4:4:4, 50 MP / 8.7 MP | 1296 MB | 276 MB |

T-113: the 371 MB row was measured with a WebP whose alpha is not palette-coded. A lossy WebP with a method-1 ALPH chunk whose alpha has 5 to 16 levels
decodes with 8.5 bytes per pixel (`TestDecodedBytesBoundsALossyWebPWithAlpha`), 203 MiB of allocation for the decode alone at 5000 x 5000,
against an estimate of 59 MiB; the estimate is now 8.75 bytes per pixel, so that size is refused and 3800 x 3800 is the largest accepted (peak heap in use
262 MiB for the whole `process`, measured with `TestMeasurePeak`).

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
