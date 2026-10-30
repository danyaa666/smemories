// Package bookdesign stores the owner's customisation of a book (E09, T-141, D-41): one options document per yearbook, chosen from the
// choices the book's html template declares in the "design" block of its manifest (internal/templates). The server validates every id on
// write and ignores stale ones on read; the web template decides what each id looks like. Format and rules: docs/templates.md,
// "Design block and book designs".
package bookdesign

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/templates"
	"github.com/danyaa666/smemories/internal/textx"
)

// Version is the only options document version.
const Version = 1

// MaxOptionsBytes is the longest options document, as sent and as stored.
const MaxOptionsBytes = 4096

// CodeInvalidDesign is the 400 for a rejected options document; the message names the field.
var CodeInvalidDesign = apperr.NewCode("ERROR_INVALID_DESIGN", apperr.Param.HTTP)

func invalid(msg string) error { return apperr.New(CodeInvalidDesign, "design options: "+msg) }

// Options is the stored document. Every key is optional; an absent key means the template's default.
type Options struct {
	V        int               `json:"v"`
	Theme    string            `json:"theme,omitempty"`
	Fonts    string            `json:"fonts,omitempty"`
	Sections *SectionOptions   `json:"sections,omitempty"`
	Texts    map[string]string `json:"texts,omitempty"`
	Images   map[string]string `json:"images,omitempty"`
}

// SectionOptions is the page order (every section once) and the hidden sections.
type SectionOptions struct {
	Order  []string `json:"order,omitempty"`
	Hidden []string `json:"hidden,omitempty"`
}

// Resolved is the design the renderer applies: every default filled in, stale ids gone. Texts holds only the customised texts
// (an absent text is the template's own default).
type Resolved struct {
	Theme    string            `json:"theme"`
	Fonts    string            `json:"fonts"`
	Sections ResolvedSections  `json:"sections"`
	Texts    map[string]string `json:"texts"`
	Images   map[string]string `json:"images"`
}

// ResolvedSections is the full page order and the hidden sections; both lists are always present.
type ResolvedSections struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
}

// wire is Options as the client sends it: the version is a pointer so that a missing one is told from 0.
type wire struct {
	V        *int              `json:"v"`
	Theme    string            `json:"theme"`
	Fonts    string            `json:"fonts"`
	Sections *SectionOptions   `json:"sections"`
	Texts    map[string]string `json:"texts"`
	Images   map[string]string `json:"images"`
}

// Validate checks raw against the template's design block and returns the document to store: texts normalised (NFC, trimmed,
// empty ones dropped), keys in a fixed order. Nothing is stored for a rejected document.
func Validate(d *templates.DesignBlock, raw []byte) ([]byte, error) {
	if len(raw) > MaxOptionsBytes {
		return nil, invalid("is larger than 4096 bytes")
	}
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return nil, invalid("must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w wire
	if err := dec.Decode(&w); err != nil {
		return nil, invalid(decodeMessage(err))
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, invalid("has data after the object")
	}
	if w.V != nil && *w.V != Version {
		return nil, invalid("v must be 1")
	}
	o := Options{V: Version}
	if w.Theme != "" {
		if !slices.Contains(d.Themes, w.Theme) {
			return nil, invalid("theme: unknown id")
		}
		o.Theme = w.Theme
	}
	if w.Fonts != "" {
		if !slices.Contains(d.FontSets, w.Fonts) {
			return nil, invalid("fonts: unknown id")
		}
		o.Fonts = w.Fonts
	}
	if err := checkSections(d, w.Sections); err != nil {
		return nil, err
	}
	if w.Sections != nil && (w.Sections.Order != nil || len(w.Sections.Hidden) > 0) {
		o.Sections = w.Sections
	}
	var err error
	if o.Texts, err = checkTexts(d, w.Texts); err != nil {
		return nil, err
	}
	if err := checkImages(d, w.Images); err != nil {
		return nil, err
	}
	if len(w.Images) > 0 {
		o.Images = w.Images
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // plain text stays readable in the table; the web renders it as text, never as markup
	if err := enc.Encode(&o); err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "encode design options")
	}
	out := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if len(out) > MaxOptionsBytes {
		return nil, invalid("is larger than 4096 bytes")
	}
	return out, nil
}

// decodeMessage names the field of a type error; any other decoding error is reported by its own text.
func decodeMessage(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return te.Field + ": wrong type, expected " + te.Type.String()
	}
	if msg, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return "unknown key " + msg
	}
	return "is not valid JSON"
}

func checkSections(d *templates.DesignBlock, s *SectionOptions) error {
	if s == nil {
		return nil
	}
	if s.Order != nil {
		if len(s.Order) != len(d.Sections) {
			return invalid("sections.order: must list every section exactly once")
		}
		seen := map[string]bool{}
		for i, id := range s.Order {
			sec := slices.IndexFunc(d.Sections, func(x templates.DesignSection) bool { return x.ID == id })
			switch {
			case sec < 0:
				return invalid("sections.order: unknown id")
			case seen[id]:
				return invalid("sections.order: " + id + " is listed twice")
			case !d.Sections[i].Movable && d.Sections[i].ID != id:
				return invalid("sections.order: position " + strconv.Itoa(i+1) + " belongs to the fixed section " + d.Sections[i].ID)
			}
			seen[id] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range s.Hidden {
		i := slices.IndexFunc(d.Sections, func(x templates.DesignSection) bool { return x.ID == id })
		switch {
		case i < 0:
			return invalid("sections.hidden: unknown id")
		case !d.Sections[i].Hideable:
			return invalid("sections.hidden: " + id + " cannot be hidden")
		case seen[id]:
			return invalid("sections.hidden: " + id + " is listed twice")
		}
		seen[id] = true
	}
	return nil
}

// checkTexts returns the texts to store: cleaned, and without the empty ones (empty means the default).
func checkTexts(d *templates.DesignBlock, in map[string]string) (map[string]string, error) {
	var out map[string]string
	for id, v := range in {
		i := slices.IndexFunc(d.Texts, func(x templates.DesignText) bool { return x.ID == id })
		if i < 0 {
			return nil, invalid("texts: unknown id")
		}
		s, ok := textx.Clean(v, 0, d.Texts[i].Max)
		if !ok {
			return nil, invalid("texts." + id + ": must be at most " + strconv.Itoa(d.Texts[i].Max) + " characters, one line, no control characters")
		}
		if s == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[id] = s
	}
	return out, nil
}

func checkImages(d *templates.DesignBlock, in map[string]string) error {
	for id, choice := range in {
		i := slices.IndexFunc(d.Images, func(x templates.DesignImage) bool { return x.ID == id })
		if i < 0 {
			return invalid("images: unknown id")
		}
		if !slices.Contains(d.Images[i].Choices, choice) {
			return invalid("images." + id + ": unknown choice")
		}
	}
	return nil
}

// Resolve applies stored (the options_json of the book's row, or nil) to the design block: a missing or unreadable document is the
// defaults, and an id the manifest no longer has falls back to the default silently, so an old design never breaks a print.
func Resolve(d *templates.DesignBlock, stored []byte) *Resolved {
	var o Options
	if stored != nil && json.Unmarshal(stored, &o) != nil {
		o = Options{}
	}
	r := &Resolved{Theme: d.Themes[0], Fonts: d.FontSets[0], Texts: map[string]string{}, Images: map[string]string{}}
	if slices.Contains(d.Themes, o.Theme) {
		r.Theme = o.Theme
	}
	if slices.Contains(d.FontSets, o.Fonts) {
		r.Fonts = o.Fonts
	}
	var order, hidden []string
	if o.Sections != nil {
		order, hidden = o.Sections.Order, o.Sections.Hidden
	}
	r.Sections = ResolvedSections{Order: resolveOrder(d, order), Hidden: []string{}}
	for _, s := range d.Sections {
		if s.Hideable && slices.Contains(hidden, s.ID) {
			r.Sections.Hidden = append(r.Sections.Hidden, s.ID)
		}
	}
	for _, t := range d.Texts {
		if s, ok := textx.Clean(o.Texts[t.ID], 1, t.Max); ok {
			r.Texts[t.ID] = s
		}
	}
	for _, im := range d.Images {
		r.Images[im.ID] = im.Choices[0]
		if c := o.Images[im.ID]; slices.Contains(im.Choices, c) {
			r.Images[im.ID] = c
		}
	}
	return r
}

// resolveOrder keeps every non-movable section at its manifest position and fills the other positions with the movable ones in
// the stored order (unknown and repeated ids skipped, sections the stored order lacks added in manifest order).
func resolveOrder(d *templates.DesignBlock, stored []string) []string {
	movable := map[string]bool{}
	var seq []string
	for _, s := range d.Sections {
		movable[s.ID] = s.Movable
	}
	for _, id := range stored {
		if movable[id] && !slices.Contains(seq, id) {
			seq = append(seq, id)
		}
	}
	for _, s := range d.Sections {
		if s.Movable && !slices.Contains(seq, s.ID) {
			seq = append(seq, s.ID)
		}
	}
	out := make([]string, len(d.Sections))
	for i, s := range d.Sections {
		if s.Movable {
			out[i], seq = seq[0], seq[1:]
		} else {
			out[i] = s.ID
		}
	}
	return out
}
