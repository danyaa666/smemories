# Owner tests: browser print on real devices (T-054, ADR 0003, D-24)

Safari, Android Chrome and iOS Safari are untested. If one fails, ADR 0003 Option B (headless Chromium on the server) is used for that device class. About 30 minutes in total.

## Setup

On the computer: `cd web && npm ci && npm run dev -- --host`, then open
`http://localhost:5173/spike/print?design=memory&size=a5&lang=vi`.
From a phone on the same Wi-Fi use `http://<computer-ip>:5173/spike/print?design=memory&size=a5&lang=vi`.
Other pages: `design=classic`, `size=a4` or `size=letter`, `lang=en`.

Press "Save as PDF" and record, for each browser:

- paper size offered, margins, headers and footers on by default
- background graphics option: does the page print its colours when it is off?
- page count (expected 24 for memory, 4 or 5 for classic) and any blank last page
- photo sharpness (zoom the PDF to 400 %) and file size
- seconds from the click to the preview, and whether the tab survives
- whether the Vietnamese text can be selected and pasted
- whether the dotted background looks right

## Steps

1. **Chrome and Edge (desktop)**: Ctrl/Cmd+P (or the button), Destination "Save as PDF", More settings: Paper size should already show A5 (if not, that is the finding), Margins "None" or "Default", Headers and footers off, Background graphics off, then on.
2. **Firefox (desktop)**: File > Print, destination "Save to PDF", paper size A5, Margins None, "Print backgrounds" on. Look for the black dotted background (known finding) and the file size.
3. **Safari (desktop)**: File > Print, PDF button at the lower left > "Save as PDF"; Paper size A5; tick "Print backgrounds" in the print panel (off by default); check page count and background.
4. **Android Chrome**: open the URL, menu (three dots) > Share > Print, "Save as PDF"; open the paper size list (is A5 there? does the page keep its size when you pick A4?), find a background option and check the colours. Note the time and whether the page crashes with the 24-page book.
5. **iOS Safari**: Share > Print, pinch the page preview outwards (this turns it into a PDF), Share > Save to Files. Check page count, whether the paper size is Letter whatever `@page` asks, backgrounds, and whether the tab survives 24 pages with 30 photos (also try `size=letter`).
6. Repeat 1 and 4 with `scale=transform` appended to the URL if page counts differ.

## Result table (copy, fill in, send to the leader)

| Browser | Paper size / orientation | Margins, headers | Backgrounds | Pages | Photos sharp | Vietnamese selectable | Time | File size | Verdict |
|---|---|---|---|---|---|---|---|---|---|
| Safari desktop | | | | | | | | | |
| Android Chrome | | | | | | | | | |
| iOS Safari | | | | | | | | | |

The leader copies the results into the matrix of [ADR 0003](../../adr/0003-html-print-export.md).
