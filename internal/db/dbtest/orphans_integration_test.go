//go:build integration

package dbtest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// recorder is a testing.TB that remembers Errorf messages instead of failing the real test.
type recorder struct {
	testing.TB
	errs []string
}

func (r *recorder) Errorf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatalf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recorder) Helper()                   {}

// QA T-065 AC4: AssertNoOrphans is silent on a clean (and on a NULL-reference) database and names the pair when a child row
// refers to a parent row that does not exist.
func TestQAAssertNoOrphansCatchesPlantedOrphans(t *testing.T) {
	d := dbtest.New(t)
	exec := func(q string) {
		t.Helper()
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	check := func(want string) {
		t.Helper()
		r := &recorder{TB: t}
		dbtest.AssertNoOrphans(r, d)
		got := strings.Join(r.errs, "\n")
		if want == "" && got != "" {
			t.Fatalf("clean database flagged: %s", got)
		}
		if want != "" && !strings.Contains(got, want) {
			t.Fatalf("planted orphan %q not reported; got %q", want, got)
		}
	}
	check("")
	exec(`INSERT INTO users (public_id, email, display_name, created_at, updated_at) VALUES ('U1', 'a@example.com', 'A', NOW(6), NOW(6))`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B1', id, 'One', 'en', 1, 1 FROM users`)
	exec(`INSERT INTO profile_tab (public_id, yearbook_id, is_owner, full_name, created_at, updated_at) SELECT 'P1', id, 1, 'N', 1, 1 FROM yearbook_tab`)
	exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		SELECT 'M1', id, 'owner', 'k', 't', 'image/png', 1, 1, 1, REPEAT('a', 64), 1, 1 FROM yearbook_tab`)
	check("") // NULL cover and photo references are not orphans

	for _, c := range []struct{ plant, undo, want string }{
		{`UPDATE yearbook_tab SET cover_media_id = 4242`, `UPDATE yearbook_tab SET cover_media_id = NULL`, "yearbook_tab.cover_media_id"},
		{`UPDATE profile_tab SET photo_media_id = 4242`, `UPDATE profile_tab SET photo_media_id = NULL`, "profile_tab.photo_media_id"},
		{`UPDATE yearbook_tab SET owner_id = 4242`, `UPDATE yearbook_tab SET owner_id = (SELECT id FROM users)`, "yearbook_tab.owner_id"},
		{`UPDATE profile_tab SET yearbook_id = 4242`, `UPDATE profile_tab SET yearbook_id = (SELECT id FROM yearbook_tab)`, "profile_tab.yearbook_id"},
		{`UPDATE media_tab SET yearbook_id = 4242`, `UPDATE media_tab SET yearbook_id = (SELECT id FROM yearbook_tab)`, "media_tab.yearbook_id"},
	} {
		exec(c.plant)
		check(c.want)
		exec(c.undo)
		check("")
	}
}
