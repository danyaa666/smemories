//go:build integration

package media

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
)

// T-104: a panic while loading a contributor's photo, through the real save path, is a 400 for the caller, leaves the
// slot gauge at 0, is counted once, and stores nothing; the next upload works.
func TestPanickingLoaderLeavesNoSlotAndNoRow(t *testing.T) {
	e := newEnv(t)
	u := e.register("owner@example.com")
	book := e.newBook(u)
	var row uint64
	if err := e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&row); err != nil {
		t.Fatal(err)
	}
	panics := 0
	e.svc.SetPanicReporter(nil, func() { panics++ })

	_, err := e.svc.UploadContributorFrom(context.Background(), row, func() ([]byte, error) { panic("boom") })
	if c := apperr.CodeOf(err); c != CodeInvalidImage || c.HTTP != 400 {
		t.Fatalf("err = %v, want ERROR_INVALID_IMAGE 400", err)
	}
	if n := e.svc.SlotsInUse(); n != 0 || panics != 1 {
		t.Fatalf("slots in use = %d (want 0), panics counted = %d (want 1)", n, panics)
	}
	if e.count(`SELECT COUNT(*) FROM media_tab`) != 0 || e.st.count(yearbookPrefix(book)) != 0 {
		t.Fatal("a panicking upload stored something")
	}
	for range 3 { // more attempts than there are slots: a leak would make the next one wait forever
		_, _ = e.svc.UploadContributorFrom(context.Background(), row, func() ([]byte, error) { panic("boom") })
	}
	if _, err := e.svc.UploadContributor(context.Background(), row, photo(t)); err != nil {
		t.Fatalf("upload after panics: %v", err)
	}
}
