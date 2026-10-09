# Self-hosted fonts (spike T-054)

Subsets (latin, latin-ext, vietnamese where the family has one) of five Google Fonts families, all under the SIL Open Font
License 1.1: Nunito, Fredoka, Gaegu, Cormorant Garamond, Jost. They are the families used by the canvases temp1 and temp2.
The files were downloaded once from `fonts.gstatic.com` (the canvas bundles reference the same files) and are served from
this folder; the prototype makes no request to Google. Dev-only: the production build does not include them.

Vietnamese coverage of the families (148 Vietnamese letters checked with fontTools): Nunito and Cormorant Garamond 148/148,
Jost 56, Fredoka 46, Gaegu 14. See `docs/adr/0003-html-print-export.md`.
