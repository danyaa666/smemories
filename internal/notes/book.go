package notes

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/danyaa666/smemories/internal/signature"
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
	// Signature is the note's vector signature (E11), null = none. Only integers: it is parsed again on the way out.
	Signature *signature.Signature `json:"signature"`
}

// decodeSignature turns the stored column into the value the API returns: nil for NULL and for a stored string that is not a valid
// signature (a damaged row shows no signature rather than unchecked data).
func decodeSignature(s sql.NullString) *signature.Signature {
	if !s.Valid {
		return nil
	}
	return signature.Decode(s.String)
}

// ApprovedNotes returns the approved notes of the owner's book in book order (at most the cap) and the real number of
// approved notes, which is larger when the book was cut. Two queries whatever the number of notes, plus one count when the
// cap was reached. A book that is not the owner's has no rows here: the caller checks ownership of the book itself.
func (s *Service) ApprovedNotes(ctx context.Context, ownerID uint64, verified bool, bookID string) ([]*BookNote, int, error) {
	if err := checkOwner(verified, bookID); err != nil {
		return nil, 0, err
	}
	return s.mod.approvedNotes(ctx, ownerID, bookID, maxBookNotes)
}

// MaxClassBookNotes is the most approved notes one student contributes to the class book (E07, T-027): 60 members x 40 notes keeps
// the class book near the 120-page budget. The cut is never silent: the caller gets the real approved count next to the notes.
const MaxClassBookNotes = 40

// ClassBookNotes is ApprovedNotes for the class book: the same rows in the same order (sort_order, then creation) and the same
// query, cut at MaxClassBookNotes. The caller is the class service, which passes the book's own owner and public id from the class
// member row; the verified-email rule is the book owner's, not the class admin's, so it is not checked here.
func (s *Service) ClassBookNotes(ctx context.Context, ownerID uint64, bookID string) ([]*BookNote, int, error) {
	return s.mod.approvedNotes(ctx, ownerID, bookID, MaxClassBookNotes)
}

// MaxExportNotes bounds the text export of a book (T-093): more than the book-data cap, because no browser has to draw it.
const MaxExportNotes = 1000

// ExportNotes is ApprovedNotes with the export cap: the same rows in the same order and the same two queries.
func (s *Service) ExportNotes(ctx context.Context, ownerID uint64, verified bool, bookID string) ([]*BookNote, int, error) {
	if err := checkOwner(verified, bookID); err != nil {
		return nil, 0, err
	}
	return s.mod.approvedNotes(ctx, ownerID, bookID, MaxExportNotes)
}

func (s *Store) approvedNotes(ctx context.Context, ownerID uint64, bookID string, limit int) ([]*BookNote, int, error) {
	out, err := s.approvedPage(ctx, ownerID, bookID, limit)
	if err != nil || len(out) < limit {
		return out, len(out), err
	}
	var total int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM note_tab n JOIN yearbook_tab y ON y.id = n.yearbook_id WHERE y.owner_id = ? AND y.public_id = ? AND n.status = ?`,
		ownerID, bookID, StatusApproved).Scan(&total)
	return out, max(total, len(out)), err
}

func (s *Store) approvedPage(ctx context.Context, ownerID uint64, bookID string, limit int) ([]*BookNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.id, n.public_id, n.answers, n.signature_json FROM note_tab n JOIN yearbook_tab y ON y.id = n.yearbook_id
		 WHERE y.owner_id = ? AND y.public_id = ? AND n.status = ? ORDER BY n.sort_order, n.id LIMIT ?`,
		ownerID, bookID, StatusApproved, limit)
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
		var sig sql.NullString
		if err := rows.Scan(&row, &n.ID, &answers, &sig); err != nil {
			return nil, err
		}
		n.Answers = answers
		n.Signature = decodeSignature(sig)
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
