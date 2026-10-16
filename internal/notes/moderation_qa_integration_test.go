//go:build integration

package notes

import (
	"strings"
	"testing"
)

// QA (T-013): the column collation is case-insensitive, so a note id typed in another case matches the row. The batch must
// not take that for a different id and dereference a missing map entry (500 ERROR_INTERNAL): it answers a 4xx or treats it as the note.
func TestQAMixedCaseNoteIDIsNeverInternal(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 2, nil)
	lower := strings.ToLower(ids[0])
	for _, status := range []string{StatusApproved, StatusHidden, StatusPending} {
		r := e.setStatus(l.owner, l.book, []string{lower}, status)
		if r.Code >= 500 {
			t.Fatalf("batch-update-status %s with a lower-case id: HTTP %d %s", status, r.Code, r.Body.String())
		}
	}
	if r := e.reorder(l.owner, l.book, []string{lower}); r.Code >= 500 {
		t.Fatalf("reorder with a lower-case id: HTTP %d", r.Code)
	}
	if r := e.deleteNotes(l.owner, l.book, []string{strings.ToLower(ids[1])}); r.Code >= 500 {
		t.Fatalf("batch-delete with a lower-case id: HTTP %d", r.Code)
	}
}
