# Self-hosted fonts for the book renderer (spike T-054, T-058)

Subsets (latin, latin-ext, vietnamese where the family has one) of five Google Fonts families, all under the SIL Open Font
License 1.1: Nunito, Fredoka, Gaegu, Cormorant Garamond, Jost. They are the families used by the canvases temp1 and temp2.
The files were downloaded once from `fonts.gstatic.com` (the canvas bundles reference the same files) and are served from
this folder; the book renderer makes no request to Google. They are part of the book screen's chunk only (T-058).

Vietnamese coverage of the families (148 Vietnamese letters checked with fontTools): Nunito and Cormorant Garamond 148/148,
Jost 56, Fredoka 46, Gaegu 14. See `docs/adr/0003-html-print-export.md`.

## Vietnamese-capable lookalikes (T-059)

Fredoka (rounded titles) and Gaegu (handwriting) of the memory-book canvas have no usable Vietnamese (46 and 14 of 148 letters), so
the `memory-book` template uses lookalikes that cover all of it. Same source and method as above (Google Fonts, latin, latin-ext and
vietnamese subsets, woff2, served from this folder). Licence texts: `OFL-Baloo2.txt`, `OFL-PatrickHand.txt` (SIL OFL 1.1).

| Family                                 | Replaces | Source                                                      | Files                                            | Vietnamese letters (fontTools: every letter with its tone marks, both cases) | Licence                        |
| -------------------------------------- | -------- | ----------------------------------------------------------- | ------------------------------------------------ | ---------------------------------------------------------------------------- | ------------------------------ |
| Baloo 2 (variable, weights 400 to 800) | Fredoka  | fonts.google.com/specimen/Baloo+2, EkType                   | `Baloo2-{latin,latin-ext,vietnamese}.woff2`      | 148                                                                          | OFL 1.1, `OFL-Baloo2.txt`      |
| Patrick Hand (400)                     | Gaegu    | fonts.google.com/specimen/Patrick+Hand, Patrick Wagesreiter | `PatrickHand-{latin,latin-ext,vietnamese}.woff2` | 148                                                                          | OFL 1.1, `OFL-PatrickHand.txt` |

Both are used for English and Vietnamese books alike, so a Vietnamese name in an English book does not fall back letter by letter.
Fredoka and Gaegu stay in this folder for the first designs; nothing loads them unless a template names them.

## Be Vietnam Pro (T-060)

The `navy-classic` template (canvas temp2) sets its small capital labels in Jost, which has 56 of the 148 Vietnamese letters. Its
lookalike with full Vietnamese coverage is Be Vietnam Pro (geometric sans, a little wider than Jost). Cormorant Garamond, the other
family of the canvas, already covers all of it. Same source and method as above.

| Family               | Replaces | Source                                                                | Files                                                 | Vietnamese letters (every letter with its tone marks, both cases) | Licence                         |
| -------------------- | -------- | --------------------------------------------------------------------- | ----------------------------------------------------- | ----------------------------------------------------------------- | ------------------------------- |
| Be Vietnam Pro (500) | Jost     | fonts.google.com/specimen/Be+Vietnam+Pro, Vietnam Pro Project Authors | `BeVietnamPro-500-{latin,latin-ext,vietnamese}.woff2` | all 146 (fontTools: 12 vowels x 6 tones x 2 cases, plus đ Đ)      | OFL 1.1, `OFL-BeVietnamPro.txt` |

## Font set `classic` of memory-book (T-142)

The `classic` font set of the memory-book (E09) is Cormorant Garamond (titles and handwriting lines, 500/600/700 and 500/600 italic) over Be Vietnam Pro (body). Both are shipped above with all of the
Vietnamese alphabet, so no file was added; they are fetched only when a book uses that set (`memory.css`, `data-fonts="classic"`).
