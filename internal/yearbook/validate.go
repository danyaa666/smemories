package yearbook

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/templates"
	"github.com/danyaa666/smemories/internal/textx"
)

// Limits in characters (AC5).
const (
	maxTitle      = 120
	maxSchool     = 120
	maxClass      = 120
	maxMotto      = 200
	maxQuote      = 500
	maxHobbies    = 300
	maxFuture     = 300
	maxFullName   = 100
	maxNickname   = 50
	minGradYear   = 1950
	maxGradYear   = 2100
	minBirthYear  = 1900 // MySQL DATE reaches further back, but nobody in a yearbook was born before this
	dateLayout    = "2006-01-02"
	maxBooksOwned = 20
	day           = 24 * time.Hour
)

// Accepted values of the VARCHAR columns language and page_size (no database ENUM: a new value needs no ALTER).
var (
	languages = []string{"en", "vi"}
	pageSizes = []string{"A5", "A4", "Letter"}
)

// invalid is the 400 for a rejected field: the business code ERROR_INVALID_<FIELD>, which the web shows under that field.
func invalid(field string) error {
	return apperr.New(apperr.NewCode("ERROR_INVALID_"+strings.ToUpper(field), apperr.Param.HTTP), field+" is not valid")
}

// maxTemplateID is the size of the template_id column.
const maxTemplateID = 32

// optional distinguishes an absent JSON field (Set false) from an explicit null (Set true,
// Val nil), so PATCH can clear graduation_year.
type optional[T any] struct {
	Set bool
	Val *T
}

func (o *optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Val = nil
		return nil
	}
	o.Val = new(T)
	return json.Unmarshal(b, o.Val)
}

// BookInput holds the fields of yearbook/create and yearbook/update: every field is
// optional here; create then insists on title and language. A JSON null counts as absent
// except for graduation_year, cover_media_id and template_id, where it clears the value.
type BookInput struct {
	Title          *string          `json:"title"`
	SchoolName     *string          `json:"school_name"`
	ClassName      *string          `json:"class_name"`
	GraduationYear optional[int]    `json:"graduation_year"`
	Motto          *string          `json:"motto"`
	Language       *string          `json:"language"`
	PageSize       *string          `json:"page_size"`
	CoverMediaID   optional[string] `json:"cover_media_id"`
	TemplateID     optional[string] `json:"template_id"`
}

// text cleans *in (when present) with the shared rules and stores it in dst.
func text(in *string, dst *string, min, max int, field string) error {
	if in == nil {
		return nil
	}
	s, ok := textx.Clean(*in, min, max)
	if !ok {
		return invalid(field)
	}
	*dst = s
	return nil
}

// applyTo validates every field present in in and writes it to y; y is only meaningful when
// the returned error is nil.
func (in BookInput) applyTo(y *Yearbook) error {
	for _, f := range []struct {
		in       *string
		dst      *string
		min, max int
		field    string
	}{
		{in.Title, &y.Title, 1, maxTitle, "title"},
		{in.SchoolName, &y.SchoolName, 0, maxSchool, "school_name"},
		{in.ClassName, &y.ClassName, 0, maxClass, "class_name"},
		{in.Motto, &y.Motto, 0, maxMotto, "motto"},
	} {
		if err := text(f.in, f.dst, f.min, f.max, f.field); err != nil {
			return err
		}
	}
	if in.GraduationYear.Set {
		if v := in.GraduationYear.Val; v == nil {
			y.GraduationYear = nil
		} else if *v < minGradYear || *v > maxGradYear {
			return invalid("graduation_year")
		} else {
			y.GraduationYear = v
		}
	}
	if in.Language != nil {
		if !slices.Contains(languages, *in.Language) {
			return invalid("language")
		}
		y.Language = *in.Language
	}
	if in.CoverMediaID.Set {
		y.CoverMediaID = in.CoverMediaID.Val // existence and ownership are checked by the store
	}
	if in.PageSize != nil {
		if !slices.Contains(pageSizes, *in.PageSize) {
			return invalid("page_size")
		}
		y.PageSize = *in.PageSize
	}
	return in.applyTemplate(y)
}

// applyTemplate sets or clears the template and checks the result against the registry: the template must exist and
// support the book's page size (the size after this change). A page-size change on a book whose chosen template does not
// support the new size is refused too, so a book never ends with a template it cannot print on. A template id the
// registry no longer knows is only refused when the owner picks it, not when the book changes size.
func (in BookInput) applyTemplate(y *Yearbook) error {
	if in.TemplateID.Set {
		id := in.TemplateID.Val
		if id == nil {
			y.TemplateID = nil
			return nil
		}
		t, ok := templates.Get(*id)
		if len(*id) > maxTemplateID || !ok || !t.Supports(y.PageSize) {
			return invalid("template")
		}
		y.TemplateID = id
		return nil
	}
	if in.PageSize != nil && y.TemplateID != nil {
		if t, ok := templates.Get(*y.TemplateID); ok && !t.Supports(y.PageSize) {
			return invalid("template")
		}
	}
	return nil
}

// ProfileInput holds the fields of yearbook/update-profile; it replaces every field, so an
// absent optional field is cleared.
type ProfileInput struct {
	FullName     string  `json:"full_name"`
	Nickname     string  `json:"nickname"`
	Birthday     *string `json:"birthday"`
	Quote        string  `json:"quote"`
	Hobbies      string  `json:"hobbies"`
	FuturePlans  string  `json:"future_plans"`
	PhotoMediaID *string `json:"photo_media_id"`
}

// applyTo validates the input and writes it to p. now decides what "past" means for the birthday.
func (in ProfileInput) applyTo(p *Profile, now time.Time) error {
	var out Profile
	for _, f := range []struct {
		in       string
		dst      *string
		min, max int
		field    string
	}{
		{in.FullName, &out.FullName, 1, maxFullName, "full_name"},
		{in.Nickname, &out.Nickname, 0, maxNickname, "nickname"},
		{in.Quote, &out.Quote, 0, maxQuote, "quote"},
		{in.Hobbies, &out.Hobbies, 0, maxHobbies, "hobbies"},
		{in.FuturePlans, &out.FuturePlans, 0, maxFuture, "future_plans"},
	} {
		if err := text(&f.in, f.dst, f.min, f.max, f.field); err != nil {
			return err
		}
	}
	if in.Birthday != nil {
		d, err := time.Parse(dateLayout, *in.Birthday)
		if err != nil || d.Format(dateLayout) != *in.Birthday || d.Year() < minBirthYear || !d.Before(now.UTC().Truncate(day)) {
			return invalid("birthday")
		}
		s := d.Format(dateLayout)
		out.Birthday = &s
	}
	p.FullName, p.Nickname, p.Birthday, p.Quote, p.Hobbies, p.FuturePlans = out.FullName, out.Nickname, out.Birthday, out.Quote, out.Hobbies, out.FuturePlans
	p.PhotoMediaID = in.PhotoMediaID
	return nil
}
