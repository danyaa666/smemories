// Package fonts embeds the OFL fonts used by the PDF renderer (see README.md in this directory).
package fonts

import _ "embed" // for go:embed

// Primary is Be Vietnam Pro, full Vietnamese coverage.
//
//go:embed BeVietnamPro-Regular.ttf
var Regular []byte

//go:embed BeVietnamPro-Bold.ttf
var Bold []byte

// Emoji is the prepared monochrome Noto Emoji (static instance with BMP aliases for U+1F000..U+1FAFF);
// do not swap in an unmodified upstream file (ADR 0002).
//
//go:embed NotoEmoji-Regular.ttf
var Emoji []byte
