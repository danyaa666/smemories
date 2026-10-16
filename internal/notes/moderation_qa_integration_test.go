//go:build integration

package notes

import (
	"strings"
	"testing"
)

// QA (T-013): the column collation is case-insensitive, so a note id typed in another case matches the row. Ids are
// canonical upper-case ULIDs: any other spelling is ERROR_PARAM on every route (it was a 500 nil dereference), and nothing changes.
func TestQAMixedCaseNoteIDIsParamError(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 2, nil)
	lower := strings.ToLower(ids[0])
	for _, status := range []string{StatusApproved, StatusHidden, StatusPending} {
		e.setStatus(l.owner, l.book, []string{lower}, status).status(400, "ERROR_PARAM")
	}
	e.setStatus(l.owner, l.book, []string{ids[0], lower}, StatusApproved).status(400, "ERROR_PARAM") // the same note twice
	e.reorder(l.owner, l.book, []string{lower}).status(400, "ERROR_PARAM")
	e.deleteNotes(l.owner, l.book, []string{strings.ToLower(ids[1])}).status(400, "ERROR_PARAM")
	e.noteList(l.owner, strings.ToLower(l.book), "").status(400, "ERROR_PARAM")
	e.setStatus(l.owner, strings.ToLower(l.book), []string{ids[0]}, StatusApproved).status(400, "ERROR_PARAM")
	if c := e.counts(e.noteList(l.owner, l.book, "")); c != [4]int{2, 2, 0, 0} {
		t.Fatalf("counts after rejected calls = %v, want all 2 pending", c)
	}
	e.setStatus(l.owner, l.book, []string{ids[0]}, StatusApproved).status(200, "")
}
