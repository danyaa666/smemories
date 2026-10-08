# Photo processing: memory budget

Every upload is decoded, scaled to at most 3000 px (thumbnail 480 px) and re-encoded by `process` in
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

Photos sent through a collection link (`media.Service.UploadContributor`, T-034) use the same processing slots and limits. A submission with
photos holds its raw bytes (at most 3 x `SMEM_MEDIA_MAX_BYTES`) in memory until each photo is processed, one at a time; at most 2 x the slot
count of such submissions are in flight (`503 busy`), and a photo that finds no free processing slot for 5 seconds also answers `503 busy`.

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
