# Photo processing: memory budget

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

Photos sent through a collection link (`media.Service.UploadContributorFrom`, T-034) use the same processing slots and limits, but the upload
does not hold one. The whole body (at most 32 MiB) is first **spooled to a temporary file** while the phone sends it, so a slow client costs a
socket and some disk, never memory and never a processing slot. Only when the body is complete are the text validated and the photos
processed, one at a time: each photo is read from the file into memory **after** a processing slot was obtained (at most
`SMEM_MEDIA_MAX_BYTES` per slot) and the request waits at most 10 s for a slot, otherwise `503 busy`.

| Setting | Default | Meaning |
|---|---|---|
| `SMEM_PUBLIC_UPLOAD_MAX_CONNS` | 48 | public note submissions in progress at once (all addresses); the next one is `503 busy` |
| `SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP` | 8 | ... of which one client IP may have this many |
| `SMEM_UPLOAD_TMP_DIR` | OS temp dir | where the bodies are spooled (files `0600`, `smem-upload-*`, removed when the request ends, also on error, panic and disconnect) |

A refused submission (`503 busy`, 2 to 6 s random `Retry-After`) is answered before any byte of its body is read and is not counted against
the per-IP request limits, so a class that retries cannot lock itself out. A body must keep moving: no byte for 10 s, or less than 16 KiB/s on
average after a 15 s grace period, or more than 120 s in total is dropped with `408`. Early rejections (closed link, bad text, rate limit) are
sent only after the rest of the body has been read (same limits), so a browser shows the answer instead of a connection reset.

**Disk sizing.** Worst case is `SMEM_PUBLIC_UPLOAD_MAX_CONNS` x 32 MiB = 1.5 GiB of temporary files (48 x 32 MiB), in practice far less. In production
(T-022) the ephemeral storage of the task, or the volume behind `SMEM_UPLOAD_TMP_DIR`, must exceed that plus headroom. A process that is killed (or whose drain
is cut off) leaves its files behind: at start-up, before the listener accepts traffic, the API deletes every regular file named `smem-upload-*`
directly in the spool directory (`SMEM_UPLOAD_TMP_DIR`, else the OS temp dir) that is older than 4 min (the 3 min route timeout plus 1 minute), so a live
upload is never touched; sub-directories and symlinks are left alone, a failure is only a WARN, and one INFO line gives the count removed (T-074).
Ephemeral storage is cleared on restart anyway. Many-address floods are the job of the WAF and load balancer limits (T-031).

## Print size (T-057, decision D-24)

Three objects per photo: `<id>.<ext>` (display, 3000 px), `<id>-print.<ext>` (print, long edge 1800 px, JPEG quality 85, PNG with
transparency stays PNG) and `<id>-thumb.jpg` (480 px). `media.print_key` holds the print key (migration 0013, NULL for photos older than
the migration). `GET /v1/media/{id}/content?size=print` serves it with the headers and authorisation of the other sizes (ETag ends in `-p`);
while `print_key` is NULL it answers in display size, so the print renderer works before and during the backfill.

- A photo whose long edge is at most 1800 px keeps its display bytes as the print object (no second lossy pass).
- The thumbnail is scaled from the print size (cheaper than from 3000 px); the print version is scaled and encoded in a goroutine
  while the display is encoded. Process time of a 4032 x 3024 JPEG, median of 9 on a laptop: 0.90 s before, 0.86 s after. On a single
  core the extra work is about 20%. Peak memory with the two largest accepted inputs (49 MP JPEG, 25 MP PNG), 2 at a time: 954 MiB
  before, 960 MiB after (the print buffer is 1800 x 1350 x 4 bytes, about 10 MB).
- Deleting a photo, discarding a failed submission or deleting a yearbook removes the print object too; a failed print write leaves nothing behind.

**Backfill.** `bin/smemories-media-backfill [--dry-run] [--batch 100]` (same `SMEM_*` environment as the API: database and S3) creates the print object
of every photo with `print_key IS NULL` from its stored display object, oldest first. It is idempotent (finished photos are never selected; a crash only
repeats one write), can be stopped with Ctrl-C and restarted, and holds one image in memory at a time. It prints
`print objects created: N, skipped: N, failed: N` (skipped: deleted meanwhile; failed: display object missing or unreadable, logged by media id, retried by
the next run) and exits 1 when something failed. Run `--dry-run` first to see how many photos are waiting. Run it once after `make migrate` / the deploy
that carries migration 0013; locally `make build && bin/smemories-media-backfill`.

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
