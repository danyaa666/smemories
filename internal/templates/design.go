package templates

import (
	"fmt"
	"regexp"
)

// Fixed positions of a section in a design (Design.Sections[i].Fixed).
const (
	FixedFirst = "first"
	FixedLast  = "last"
)

// Limits of a design block (E09, T-141). The options document of a book is validated against them (internal/bookdesign).
const (
	MaxThemes      = 8
	MaxFontSets    = 6
	MaxSections    = 16
	MaxTexts       = 16
	MaxImages      = 8
	MaxImageChoice = 8
	MaxTextRunes   = 200
)

// designIDRe is the shape of every id in a design block (theme, font set, section, text, image and choice ids).
var designIDRe = regexp.MustCompile(`^[a-z0-9._-]{1,40}$`)

// DesignBlock is the optional "design" block of an html manifest: the choices a student may make for the template.
// The web template defines what each id looks like; the first theme, font set and image choice is the default.
type DesignBlock struct {
	Themes   []string        `json:"themes"`
	FontSets []string        `json:"font_sets"`
	Sections []DesignSection `json:"sections"`
	Texts    []DesignText    `json:"texts"`
	Images   []DesignImage   `json:"images"`
}

// DesignSection is a page group the student may hide or move. Fixed ("first" or "last", or empty) pins it to an end.
type DesignSection struct {
	ID       string  `json:"id"`
	Hideable bool    `json:"hideable"`
	Movable  bool    `json:"movable"`
	Fixed    *string `json:"fixed"` // FixedFirst, FixedLast or null
}

// Pin is Fixed as a plain string: FixedFirst, FixedLast or "".
func (s DesignSection) Pin() string {
	if s.Fixed == nil {
		return ""
	}
	return *s.Fixed
}

// DesignText is a fixed text the student may replace; Max is the longest value in runes.
type DesignText struct {
	ID  string `json:"id"`
	Max int    `json:"max"`
}

// DesignImage is a picture slot with the ids of the pictures that may fill it; Choices[0] is the default.
type DesignImage struct {
	ID      string   `json:"id"`
	Choices []string `json:"choices"`
}

// Design returns the design block of a built-in template; ok is false when the template is unknown or declares none.
// The result is shared: do not modify it.
func Design(templateID string) (d *DesignBlock, ok bool) {
	t, found := registry[templateID]
	if !found || t.Design == nil {
		return nil, false
	}
	return t.Design, true
}

// design checks the design block of an html manifest.
func (v *validator) design() {
	d := v.t.Design
	if d == nil {
		return
	}
	v.idList("design.themes", d.Themes, 1, MaxThemes)
	v.idList("design.font_sets", d.FontSets, 1, MaxFontSets)
	v.sections(d.Sections)
	if len(d.Texts) > MaxTexts {
		v.errf("", "design.texts has %d entries, the limit is %d", len(d.Texts), MaxTexts)
	}
	seen := map[string]bool{}
	for _, x := range d.Texts {
		v.designID("design.texts", x.ID, seen)
		if x.Max < 1 || x.Max > MaxTextRunes {
			v.errf("", "design.texts %q: max %d must be in 1..%d", x.ID, x.Max, MaxTextRunes)
		}
	}
	if len(d.Images) > MaxImages {
		v.errf("", "design.images has %d entries, the limit is %d", len(d.Images), MaxImages)
	}
	seen = map[string]bool{}
	for _, x := range d.Images {
		v.designID("design.images", x.ID, seen)
		v.idList(fmt.Sprintf("design.images %q choices", x.ID), x.Choices, 1, MaxImageChoice)
	}
}

// designID checks one id's shape and that seen has not got it yet; it records it.
func (v *validator) designID(where, id string, seen map[string]bool) {
	switch {
	case !designIDRe.MatchString(id):
		v.errf("", "%s: id %q must match %s", where, id, designIDRe)
	case seen[id]:
		v.errf("", "%s: id %q is listed twice", where, id)
	}
	seen[id] = true
}

// idList checks a list of n unique ids with min <= n <= max.
func (v *validator) idList(where string, ids []string, lo, hi int) {
	if len(ids) < lo || len(ids) > hi {
		v.errf("", "%s needs %d..%d ids, has %d", where, lo, hi, len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		v.designID(where, id, seen)
	}
}

// sections checks the page groups: unique ids, a valid fixed value, at most one first and one last, the first
// at the front and the last at the end of the list, and a fixed section is not movable.
func (v *validator) sections(ss []DesignSection) {
	if len(ss) > MaxSections {
		v.errf("", "design.sections has %d entries, the limit is %d", len(ss), MaxSections)
	}
	seen := map[string]bool{}
	firsts, lasts := 0, 0
	for i, s := range ss {
		v.designID("design.sections", s.ID, seen)
		switch s.Pin() {
		case "":
		case FixedFirst:
			firsts++
			if i != 0 {
				v.errf("", "design.sections %q is fixed first, so it must be listed first", s.ID)
			}
		case FixedLast:
			lasts++
			if i != len(ss)-1 {
				v.errf("", "design.sections %q is fixed last, so it must be listed last", s.ID)
			}
		default:
			v.errf("", "design.sections %q: fixed %q must be %q, %q or null", s.ID, s.Pin(), FixedFirst, FixedLast)
		}
		if s.Pin() != "" && s.Movable {
			v.errf("", "design.sections %q is fixed, so it cannot be movable", s.ID)
		}
	}
	if firsts > 1 || lasts > 1 {
		v.errf("", "design.sections has more than one section fixed first or last")
	}
}
