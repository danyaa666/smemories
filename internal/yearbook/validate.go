package yearbook

import (
	"encoding/json"
	"slices"
	"time"

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
)

// pageSizes are the accepted page_size values (the yearbooks.page_size enum).
var pageSizes = []string{"A5", "A4", "Letter"}

// ValidationError carries the stable API error code (invalid_<field>) for a rejected input.
type ValidationError struct{ Code string }

func (e ValidationError) Error() string { return "yearbook: " + e.Code }

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

// bookInput is the body of POST /v1/yearbooks and PATCH /v1/yearbooks/{id}: every field is
// optional here; create then insists on title and language. A JSON null counts as absent
// except for graduation_year, where it clears the value.
type bookInput struct {
	Title          *string          `json:"title"`
	SchoolName     *string          `json:"school_name"`
	ClassName      *string          `json:"class_name"`
	GraduationYear optional[int]    `json:"graduation_year"`
	Motto          *string          `json:"motto"`
	Language       *string          `json:"language"`
	PageSize       *string          `json:"page_size"`
	CoverMediaID   optional[string] `json:"cover_media_id"`
}

// text cleans *in (when present) with the shared rules and stores it in dst.
func text(in *string, dst *string, min, max int, code string) error {
	if in == nil {
		return nil
	}
	s, ok := textx.Clean(*in, min, max)
	if !ok {
		return ValidationError{code}
	}
	*dst = s
	return nil
}

// applyTo validates every field present in in and writes it to y; y is only meaningful when
// the returned error is nil.
func (in bookInput) applyTo(y *Yearbook) error {
	for _, f := range []struct {
		in       *string
		dst      *string
		min, max int
		code     string
	}{
		{in.Title, &y.Title, 1, maxTitle, "invalid_title"},
		{in.SchoolName, &y.SchoolName, 0, maxSchool, "invalid_school_name"},
		{in.ClassName, &y.ClassName, 0, maxClass, "invalid_class_name"},
		{in.Motto, &y.Motto, 0, maxMotto, "invalid_motto"},
	} {
		if err := text(f.in, f.dst, f.min, f.max, f.code); err != nil {
			return err
		}
	}
	if in.GraduationYear.Set {
		if v := in.GraduationYear.Val; v == nil {
			y.GraduationYear = nil
		} else if *v < minGradYear || *v > maxGradYear {
			return ValidationError{"invalid_graduation_year"}
		} else {
			y.GraduationYear = v
		}
	}
	if in.Language != nil {
		if *in.Language != "en" && *in.Language != "vi" {
			return ValidationError{"invalid_language"}
		}
		y.Language = *in.Language
	}
	if in.CoverMediaID.Set {
		y.CoverMediaID = in.CoverMediaID.Val // existence and ownership are checked by the store
	}
	if in.PageSize != nil {
		if !slices.Contains(pageSizes, *in.PageSize) {
			return ValidationError{"invalid_page_size"}
		}
		y.PageSize = *in.PageSize
	}
	return nil
}

// profileInput is the body of PUT /v1/yearbooks/{id}/profile; it replaces every field, so an
// absent optional field is cleared.
type profileInput struct {
	FullName     string  `json:"full_name"`
	Nickname     string  `json:"nickname"`
	Birthday     *string `json:"birthday"`
	Quote        string  `json:"quote"`
	Hobbies      string  `json:"hobbies"`
	FuturePlans  string  `json:"future_plans"`
	PhotoMediaID *string `json:"photo_media_id"`
}

// applyTo validates the input and writes it to p. now decides what "past" means for the birthday.
func (in profileInput) applyTo(p *Profile, now time.Time) error {
	var out Profile
	for _, f := range []struct {
		in       string
		dst      *string
		min, max int
		code     string
	}{
		{in.FullName, &out.FullName, 1, maxFullName, "invalid_full_name"},
		{in.Nickname, &out.Nickname, 0, maxNickname, "invalid_nickname"},
		{in.Quote, &out.Quote, 0, maxQuote, "invalid_quote"},
		{in.Hobbies, &out.Hobbies, 0, maxHobbies, "invalid_hobbies"},
		{in.FuturePlans, &out.FuturePlans, 0, maxFuture, "invalid_future_plans"},
	} {
		if err := text(&f.in, f.dst, f.min, f.max, f.code); err != nil {
			return err
		}
	}
	if in.Birthday != nil {
		d, err := time.Parse(dateLayout, *in.Birthday)
		if err != nil || d.Format(dateLayout) != *in.Birthday || d.Year() < minBirthYear || !d.Before(now.UTC().Truncate(24*time.Hour)) {
			return ValidationError{"invalid_birthday"}
		}
		s := d.Format(dateLayout)
		out.Birthday = &s
	}
	p.FullName, p.Nickname, p.Birthday, p.Quote, p.Hobbies, p.FuturePlans = out.FullName, out.Nickname, out.Birthday, out.Quote, out.Hobbies, out.FuturePlans
	p.PhotoMediaID = in.PhotoMediaID
	return nil
}
