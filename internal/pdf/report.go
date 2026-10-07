package pdf

// Warning codes reported in Report.Warnings. The render still succeeds for all of them.
const (
	WarnMissingGlyph  = "missing_glyph"  // a character is in no bundled font; '?' is drawn instead (Rune)
	WarnTextTruncated = "text_truncated" // text did not fit even at min_size and ends with an ellipsis
	WarnLowResolution = "low_resolution" // effective resolution below 300 DPI (MediaID)
	WarnMissingImage  = "missing_image"  // photo could not be fetched or decoded; a placeholder is drawn (MediaID)
	WarnExtraPhotos   = "extra_photos"   // a note has more photos than the template has slots for (NoteID)
)

// Warning is one non-fatal problem. Page is 1-based; Slot, NoteID, MediaID and Rune say where and what.
type Warning struct {
	Code    string
	Page    int
	Slot    string
	NoteID  string
	MediaID string
	Rune    string
	DPI     float64 // low_resolution only
}

// Report is what Render found besides the PDF itself.
type Report struct {
	Pages    int
	Warnings []Warning
}

// Has reports whether a warning with the given code exists.
func (r Report) Has(code string) bool {
	for _, w := range r.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}
