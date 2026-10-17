package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestCompareSchema(t *testing.T) {
	known := map[int64]bool{1: true, 2: true, 3: true}
	for _, tc := range []struct {
		name    string
		applied []int64
		ahead   int
		errHas  []string // all must appear in the refusal; none means no error
		errNot  string   // must not appear
	}{
		{"equal", []int64{1, 2, 3}, 0, nil, ""},
		{"never migrated", nil, 0, []string{"version 0", "expects 3", "3 migrations pending"}, ""},
		{"behind", []int64{1}, 0, []string{"version 1", "expects 3", "2 migrations pending"}, ""},
		{"ahead is compatible", []int64{1, 2, 3, 4, 5}, 2, nil, ""},
		{"too far ahead", []int64{1, 2, 3, 4, 5, 6, 7}, 4, []string{"version 7", "4 migrations ahead"}, ""},
		// AC3: the gap names the missing version and the next applied one, two different numbers.
		{"gap in the middle", []int64{1, 3}, 0, []string{"missing migration 2", "next applied one is 3"}, "version 3, this build expects 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ahead, err := compareSchema(known, tc.applied)
			if len(tc.errHas) == 0 {
				if err != nil || ahead != tc.ahead {
					t.Fatalf("got ahead=%d err=%v, want ahead=%d and no error", ahead, err, tc.ahead)
				}
				return
			}
			if err == nil {
				t.Fatal("want a refusal")
			}
			for _, w := range tc.errHas {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("%q does not contain %q", err, w)
				}
			}
			if tc.errNot != "" && strings.Contains(err.Error(), tc.errNot) {
				t.Errorf("%q contains %q", err, tc.errNot)
			}
		})
	}
}

func TestHalfAppliedHint(t *testing.T) {
	for _, n := range []uint16{1060, 1050, 1061, 1091} {
		err := halfAppliedHint(&mysql.MySQLError{Number: n, Message: "boom"})
		if err == nil || !strings.Contains(err.Error(), "half-applied") || !strings.Contains(err.Error(), "boom") {
			t.Errorf("error %d: want the hint plus the cause, got %v", n, err)
		}
	}
	plain := errors.New("other")
	if got := halfAppliedHint(plain); got != plain {
		t.Errorf("an unrelated error must pass through unchanged, got %v", got)
	}
	if got := halfAppliedHint(&mysql.MySQLError{Number: 1146}); strings.Contains(got.Error(), "half-applied") {
		t.Errorf("1146 must not get the hint: %v", got)
	}
	if halfAppliedHint(nil) != nil {
		t.Error("nil must stay nil")
	}
}
