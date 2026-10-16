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
	exec(`INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('U1', 'a@example.com', 'A', 'user-u1', 1, 1)`)
	exec(`INSERT INTO user_identity_tab (user_id, provider, subject, email, created_at, updated_at) SELECT id, 'google', 'sub-1', 'a@example.com', 1, 1 FROM user_tab`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B1', id, 'One', 'en', 1, 1 FROM user_tab`)
	exec(`INSERT INTO profile_tab (public_id, yearbook_id, is_owner, full_name, created_at, updated_at) SELECT 'P1', id, 1, 'N', 1, 1 FROM yearbook_tab`)
	exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		SELECT 'M1', id, 'owner', 'k', 't', 'image/png', 1, 1, 1, REPEAT('a', 64), 1, 1 FROM yearbook_tab`)
	exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, created_at, updated_at) SELECT 'C1', id, REPEAT('x', 32), 1, 1 FROM yearbook_tab`)
	exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, created_at, updated_at) SELECT 'N1', id, yearbook_id, '{}', 'pending', 1, 1 FROM note_collection_tab`)
	exec(`INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) SELECT n.id, m.id, 0, 1, 1 FROM note_tab n, media_tab m`)
	check("") // NULL cover and photo references are not orphans

	for _, c := range []struct{ plant, undo, want string }{
		{`UPDATE yearbook_tab SET cover_media_id = 4242`, `UPDATE yearbook_tab SET cover_media_id = NULL`, "yearbook_tab.cover_media_id"},
		{`UPDATE profile_tab SET photo_media_id = 4242`, `UPDATE profile_tab SET photo_media_id = NULL`, "profile_tab.photo_media_id"},
		{`UPDATE user_identity_tab SET user_id = 4242`, `UPDATE user_identity_tab SET user_id = (SELECT id FROM user_tab)`, "user_identity_tab.user_id"},
		{`UPDATE yearbook_tab SET owner_id = 4242`, `UPDATE yearbook_tab SET owner_id = (SELECT id FROM user_tab)`, "yearbook_tab.owner_id"},
		{`UPDATE profile_tab SET yearbook_id = 4242`, `UPDATE profile_tab SET yearbook_id = (SELECT id FROM yearbook_tab)`, "profile_tab.yearbook_id"},
		{`UPDATE media_tab SET yearbook_id = 4242`, `UPDATE media_tab SET yearbook_id = (SELECT id FROM yearbook_tab)`, "media_tab.yearbook_id"},
		{`UPDATE note_collection_tab SET yearbook_id = 4242`, `UPDATE note_collection_tab SET yearbook_id = (SELECT id FROM yearbook_tab)`, "note_collection_tab.yearbook_id"},
		{`UPDATE note_tab SET collection_id = 4242`, `UPDATE note_tab SET collection_id = (SELECT id FROM note_collection_tab)`, "note_tab.collection_id"},
		{`UPDATE note_tab SET yearbook_id = 4242`, `UPDATE note_tab SET yearbook_id = (SELECT id FROM yearbook_tab)`, "note_tab.yearbook_id"},
		{`UPDATE note_photo_tab SET note_id = 4242`, `UPDATE note_photo_tab SET note_id = (SELECT id FROM note_tab)`, "note_photo_tab.note_id"},
		{`UPDATE note_photo_tab SET media_id = 4242`, `UPDATE note_photo_tab SET media_id = (SELECT id FROM media_tab)`, "note_photo_tab.media_id"},
	} {
		exec(c.plant)
		check(c.want)
		exec(c.undo)
		check("")
	}
}
