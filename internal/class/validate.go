package class

import (
	"slices"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/textx"
)

// Limits in characters and years (the school and the year follow yearbook).
const (
	maxName     = 120
	maxSchool   = 120
	minGradYear = 1950
	maxGradYear = 2100
)

// Roles, in rising order of power. Stored as VARCHAR (no ENUM), validated here.
const (
	RoleStudent = "student"
	RoleAdmin   = "admin"
)

var (
	roles     = []string{RoleStudent, RoleAdmin}
	languages = []string{"en", "vi"}
)

// atLeast reports whether role is min or stronger. An unknown role is never enough.
func atLeast(role, min string) bool {
	r, m := slices.Index(roles, role), slices.Index(roles, min)
	return r >= 0 && m >= 0 && r >= m
}

// invalid is the 400 for a rejected field: the business code ERROR_INVALID_<FIELD>, which the web shows under that field.
func invalid(field string) error {
	return apperr.New(apperr.NewCode("ERROR_INVALID_"+strings.ToUpper(field), apperr.Param.HTTP), field+" is not valid")
}

// ClassInput is the body of class/create and (next to id) class/update. update replaces every field, so an absent
// school_name or graduation_year is cleared.
type ClassInput struct {
	Name           string `json:"name"`
	SchoolName     string `json:"school_name"`
	GraduationYear *int   `json:"graduation_year"`
	Language       string `json:"language"`
}

// clean returns the validated copy of in (text normalised and trimmed by textx).
func (in *ClassInput) clean() (*ClassInput, error) {
	var out ClassInput
	var ok bool
	if out.Name, ok = textx.Clean(in.Name, 1, maxName); !ok {
		return nil, invalid("name")
	}
	if out.SchoolName, ok = textx.Clean(in.SchoolName, 0, maxSchool); !ok {
		return nil, invalid("school_name")
	}
	if y := in.GraduationYear; y != nil {
		if *y < minGradYear || *y > maxGradYear {
			return nil, invalid("graduation_year")
		}
		out.GraduationYear = y
	}
	if !slices.Contains(languages, in.Language) {
		return nil, invalid("language")
	}
	out.Language = in.Language
	return &out, nil
}
