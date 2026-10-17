# Owner tests: browser print on real devices (T-054, ADR 0003, D-24)

Safari, Android Chrome and iOS Safari are untested. If one fails, ADR 0003 Option B (headless Chromium on the server) is used for that device class. About 30 minutes in total.

## Setup

The throwaway spike page is gone (T-058 replaced it with the real book screen), so the test uses a real book:
on the computer `make up && make migrate`, `go run ./cmd/smemories-api` and `cd web && npm ci && npm run dev -- --host`, register, create a yearbook
(choose the page size A5, A4 or Letter), collect and approve some notes (up to 300; photos make it a realistic test), then open
`http://localhost:5173/yearbooks/<id>/book` ("Preview and print" in the menu).
From a phone on the same Wi-Fi use `http://<computer-ip>:5173/...` (the API must be reachable through the same Vite proxy).

The designed template `memory-book` (T-059) is chosen with the template picker above the print button (T-061; the sample PDFs in `docs/templates/memory-book-*.pdf` show what to expect). The print screen shows the guide for your device; after printing, answer "Does the PDF look right?" (an anonymous counter per device class, `print_feedback_tab`) in addition to filling the table below.
The same holds for the second designed template `navy-classic` (T-060; samples `docs/templates/navy-classic-*.pdf`, four friends per page: 3 plus one per 4 approved notes pages); pick it in the same picker.
In Firefox look at the dotted paper and the ruled lines of the pastel boxes: the dots are one picture (not a tiled gradient) so they should no longer be black; the ruled lines are still a repeating gradient, which is untested there.

Press "Print or save as PDF" (it is enabled once the fonts and photos are loaded) and record, for each browser:

- paper size offered, margins, headers and footers on by default
- background graphics option: does the page print its colours when it is off?
- page count (expected: 3 plus one per 4 approved notes with the plain built-in layout, 3 plus one per note with `memory-book`) and any blank last page
- photo sharpness (zoom the PDF to 400 %) and file size
- seconds from the click to the preview, and whether the tab survives
- whether the Vietnamese text can be selected and pasted

## Steps

1. **Chrome and Edge (desktop)**: Ctrl/Cmd+P (or the button), Destination "Save as PDF", More settings: Paper size should already show A5 (if not, that is the finding), Margins "None" or "Default", Headers and footers off, Background graphics off, then on.
2. **Firefox (desktop)**: File > Print, destination "Save to PDF", paper size A5, Margins None, "Print backgrounds" on. Look for the black dotted background (known finding) and the file size.
3. **Safari (desktop)**: File > Print, PDF button at the lower left > "Save as PDF"; Paper size A5; tick "Print backgrounds" in the print panel (off by default); check page count and background.
4. **Android Chrome**: open the URL, menu (three dots) > Share > Print, "Save as PDF"; open the paper size list (is A5 there? does the page keep its size when you pick A4?), find a background option and check the colours. Note the time and whether the page crashes with the 24-page book.
5. **iOS Safari**: Share > Print, pinch the page preview outwards (this turns it into a PDF), Share > Save to Files. Check page count, whether the paper size is Letter whatever `@page` asks, backgrounds, and whether the tab survives 24 pages with 30 photos (also try a Letter book).

## Result table (copy, fill in, send to the leader)

| Browser | Paper size / orientation | Margins, headers | Backgrounds | Pages | Photos sharp | Vietnamese selectable | Time | File size | Verdict |
|---|---|---|---|---|---|---|---|---|---|
| Safari desktop | | | | | | | | | |
| Android Chrome | | | | | | | | | |
| iOS Safari | | | | | | | | | |

The leader copies the results into the matrix of [ADR 0003](../../adr/0003-html-print-export.md).
