package notes

import (
	"context"
	"encoding/json"
)

// maxBookNotes bounds the notes of the book-data response: the reorder cap, so the book the renderer draws is the one the
// owner arranged. (The 300-note cap is per link and a book can have several.) A larger book is never cut silently: the
// caller gets the real approved count next to the notes.
const maxBookNotes = maxReorderIDs

// BookNote is one approved note as the book renderer reads it: public ids and the answers only (T-056).
type BookNote struct {
	ID       string          `json:"id"`
	Answers  json.RawMessage `json:"answers"`
	PhotoIDs []string        `json:"photo_ids"` // media ids, fetched with media/get-content?variant=print
}

// ApprovedNotes returns the approved notes of the owner's book in book order (at most maxBookNotes) and the real number of
// approved notes, which is larger when the book was cut. Two queries whatever the number of notes, plus one count when the
// cap was reached. A book that is not the owner's has no rows here: the caller checks ownership of the book itself.
func (s *Service) ApprovedNotes(ctx context.Context, ownerID uint64, verified bool, bookID string) ([]*BookNote, int, error) {
	if err := checkOwner(verified, bookID); err != nil {
		return nil, 0, err
	}
	return s.mod.approvedNotes(ctx, ownerID, bookID)
}

func (s *Store) approvedNotes(ctx context.Context, ownerID uint64, bookID string) ([]*BookNote, int, error) {
	out, err := s.approvedPage(ctx, ownerID, bookID)
	if err != nil || len(out) < maxBookNotes {
		return out, len(out), err
	}
	var total int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM note_tab n JOIN yearbook_tab y ON y.id = n.yearbook_id WHERE y.owner_id = ? AND y.public_id = ? AND n.status = ?`,
		ownerID, bookID, StatusApproved).Scan(&total)
	return out, max(total, len(out)), err
}

func (s *Store) approvedPage(ctx context.Context, ownerID uint64, bookID string) ([]*BookNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.id, n.public_id, n.answers FROM note_tab n JOIN yearbook_tab y ON y.id = n.yearbook_id
		 WHERE y.owner_id = ? AND y.public_id = ? AND n.status = ? ORDER BY n.sort_order, n.id LIMIT ?`,
		ownerID, bookID, StatusApproved, maxBookNotes)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*BookNote{}
	byRow := map[uint64]*BookNote{}
	var noteIDs []uint64
	for rows.Next() {
		n := &BookNote{PhotoIDs: []string{}}
		var row uint64
		var answers []byte
		if err := rows.Scan(&row, &n.ID, &answers); err != nil {
			return nil, err
		}
		n.Answers = answers
		out = append(out, n)
		byRow[row] = n
		noteIDs = append(noteIDs, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	prows, err := s.db.QueryContext(ctx,
		withIn(`SELECT np.note_id, m.public_id FROM note_photo_tab np JOIN media_tab m ON m.id = np.media_id
		 WHERE np.note_id IN (IN) ORDER BY np.note_id, np.sort_order`, len(noteIDs)), anys(noteIDs)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = prows.Close() }()
	for prows.Next() {
		var note uint64
		var photo string
		if err := prows.Scan(&note, &photo); err != nil {
			return nil, err
		}
		n := byRow[note]
		n.PhotoIDs = append(n.PhotoIDs, photo)
	}
	return out, prows.Err()
}
