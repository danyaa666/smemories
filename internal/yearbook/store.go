package yearbook

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

var (
	// ErrNotFound covers a book that does not exist and one owned by someone else: callers cannot tell them apart.
	ErrNotFound     = errors.New("yearbook: not found")
	ErrLimitReached = errors.New("yearbook: book limit reached")
)

// Yearbook is the book as the API shows it. The ids of rows are never serialised (L-05); times are Unix milliseconds
// here and rendered as RFC 3339 by MarshalJSON, so the JSON is the one the clients know.
type Yearbook struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	SchoolName     string  `json:"school_name"`
	ClassName      string  `json:"class_name"`
	GraduationYear *int    `json:"graduation_year"`
	Motto          string  `json:"motto"`
	Language       string  `json:"language"`
	PageSize       string  `json:"page_size"`
	TemplateID     *string `json:"template_id"`
	CoverMediaID   *string `json:"cover_media_id"` // a media id of this yearbook (T-009)
	CreatedAt      int64   `json:"-"`              // Unix ms
	UpdatedAt      int64   `json:"-"`              // Unix ms
	Profile        Profile `json:"profile"`
	internalID     uint64
	coverRow       uint64 // media.id of the cover, 0 = none (AUTO_INCREMENT starts at 1)
}

// Profile is the owner's profile page information.
type Profile struct {
	ID           string  `json:"id"`
	IsOwner      bool    `json:"is_owner"`
	FullName     string  `json:"full_name"`
	Nickname     string  `json:"nickname"`
	Birthday     *string `json:"birthday"` // YYYY-MM-DD
	Quote        string  `json:"quote"`
	Hobbies      string  `json:"hobbies"`
	FuturePlans  string  `json:"future_plans"`
	PhotoMediaID *string `json:"photo_media_id"` // a media id of this yearbook (T-009)
	photoRow     uint64  // media.id of the photo, 0 = none
}

// Store persists yearbooks and profiles. Every query is parameterised and scoped by owner_id:
// a book is never fetched by id alone. There are no joins: a read takes the book rows, then their profiles,
// then the public ids of the media they point at, so the query count does not depend on the page size.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const bookCols = `id, public_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, cover_media_id, created_at, updated_at`

const profileCols = `yearbook_id, public_id, is_owner, full_name, nickname, birthday, quote, hobbies, future_plans, photo_media_id`

// querier is what a read needs; *sql.DB and *sql.Tx both have it.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type scanner interface{ Scan(...any) error }

func scanBook(sc scanner) (Yearbook, error) {
	var y Yearbook
	var grad sql.Null[int]
	var tmpl sql.NullString
	var cover sql.Null[uint64]
	err := sc.Scan(&y.internalID, &y.ID, &y.Title, &y.SchoolName, &y.ClassName, &grad, &y.Motto, &y.Language, &y.PageSize, &tmpl, &cover,
		&y.CreatedAt, &y.UpdatedAt)
	if err != nil {
		return Yearbook{}, err
	}
	if grad.Valid {
		y.GraduationYear = &grad.V
	}
	if tmpl.Valid {
		y.TemplateID = &tmpl.String
	}
	y.coverRow = cover.V
	return y, nil
}

func inList(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

// fill adds the owner profile and the media public ids to books: two queries (three when a book has media), whatever
// the number of books. lock makes the profile query a locking read (inside a transaction that edits the book).
func fill(ctx context.Context, q querier, books []Yearbook, lock bool) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]any, len(books))
	at := make(map[uint64]int, len(books))
	for i := range books {
		ids[i] = books[i].internalID
		at[books[i].internalID] = i
	}
	query := `SELECT ` + profileCols + ` FROM profile_tab WHERE yearbook_id IN (` + inList(len(ids)) + `) AND is_owner = 1`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := q.QueryContext(ctx, query, ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var media []any
	seen := map[uint64]bool{}
	want := func(id uint64) {
		if id != 0 && !seen[id] {
			seen[id] = true
			media = append(media, id)
		}
	}
	found := 0
	for rows.Next() {
		var bookID uint64
		var p Profile
		var bday sql.NullTime
		var photo sql.Null[uint64]
		if err := rows.Scan(&bookID, &p.ID, &p.IsOwner, &p.FullName, &p.Nickname, &bday, &p.Quote, &p.Hobbies, &p.FuturePlans, &photo); err != nil {
			return err
		}
		if bday.Valid {
			b := bday.Time.Format(dateLayout)
			p.Birthday = &b
		}
		p.photoRow = photo.V
		i := at[bookID]
		books[i].Profile = p
		found++
		want(books[i].coverRow)
		want(p.photoRow)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found != len(books) { // no foreign key guarantees the profile, so say so loudly instead of showing a half book
		return apperr.New(apperr.Internal, "yearbook without its owner profile")
	}
	if len(media) == 0 {
		return nil
	}
	mrows, err := q.QueryContext(ctx, `SELECT id, public_id FROM media WHERE id IN (`+inList(len(media))+`)`, media...)
	if err != nil {
		return err
	}
	defer func() { _ = mrows.Close() }()
	public := map[uint64]string{}
	for mrows.Next() {
		var id uint64
		var pub string
		if err := mrows.Scan(&id, &pub); err != nil {
			return err
		}
		public[id] = pub
	}
	if err := mrows.Err(); err != nil {
		return err
	}
	for i := range books {
		y := &books[i]
		if pub, ok := public[y.coverRow]; ok {
			y.CoverMediaID = &pub
		}
		if pub, ok := public[y.Profile.photoRow]; ok {
			y.Profile.PhotoMediaID = &pub
		}
	}
	return nil
}

// create inserts the book and its owner profile atomically. The owner's user row is locked
// first so two concurrent creates cannot both slip under the 20-book limit.
func (s *Store) create(ctx context.Context, ownerID uint64, y Yearbook) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var locked uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = ? FOR UPDATE`, ownerID).Scan(&locked); err != nil {
		return err
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM yearbook_tab WHERE owner_id = ?`, ownerID).Scan(&n); err != nil {
		return err
	}
	if n >= maxBooksOwned {
		return ErrLimitReached
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO yearbook_tab (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		y.ID, ownerID, y.Title, y.SchoolName, y.ClassName, y.GraduationYear, y.Motto, y.Language, y.PageSize, y.CreatedAt, y.UpdatedAt)
	if err != nil {
		return err
	}
	bookID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if err := insertProfile(ctx, tx, bookID, y); err != nil {
		return err
	}
	return tx.Commit()
}

func insertProfile(ctx context.Context, tx *sql.Tx, bookID int64, y Yearbook) error {
	p := y.Profile
	_, err := tx.ExecContext(ctx,
		`INSERT INTO profile_tab (public_id, yearbook_id, is_owner, full_name, nickname, birthday, quote, hobbies, future_plans, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, bookID, 1, p.FullName, p.Nickname, p.Birthday, p.Quote, p.Hobbies, p.FuturePlans, y.CreatedAt, y.UpdatedAt)
	return err
}

func (s *Store) get(ctx context.Context, ownerID uint64, publicID string) (Yearbook, error) {
	y, err := scanBook(s.db.QueryRowContext(ctx, `SELECT `+bookCols+` FROM yearbook_tab WHERE owner_id = ? AND public_id = ?`, ownerID, publicID))
	if errors.Is(err, sql.ErrNoRows) {
		return Yearbook{}, ErrNotFound
	}
	if err != nil {
		return Yearbook{}, err
	}
	books := []Yearbook{y}
	if err := fill(ctx, s.db, books, false); err != nil {
		return Yearbook{}, err
	}
	return books[0], nil
}

// cursor is the position after the last book of a page: books are ordered by (updated_at, public_id) descending.
type cursor struct {
	updatedAt int64 // Unix ms
	publicID  string
}

// list returns up to limit books, newest updated_at first, starting after c (nil = first page).
func (s *Store) list(ctx context.Context, ownerID uint64, c *cursor, limit int) ([]Yearbook, error) {
	q, args := `SELECT `+bookCols+` FROM yearbook_tab WHERE owner_id = ?`, []any{ownerID}
	if c != nil {
		q += ` AND (updated_at < ? OR (updated_at = ? AND public_id < ?))`
		args = append(args, c.updatedAt, c.updatedAt, c.publicID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY updated_at DESC, public_id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Yearbook
	for rows.Next() {
		y, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, y)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close() // the connection is free for the next two queries
	return out, fill(ctx, s.db, out, false)
}

// modify locks the owner's book, lets change edit it (and write the profile through the same
// transaction when it wants to), bumps updated_at to nowMs and returns the result.
//
// Lock order: yearbook_tab row (FOR UPDATE) first, then profile_tab, then media rows (share lock). Service.Delete and
// ClearMediaRefs use the same order, so concurrent edits, deletes and photo deletes wait for each other instead of
// deadlocking.
func (s *Store) modify(ctx context.Context, ownerID uint64, publicID string, nowMs int64, change func(*Yearbook) error) (Yearbook, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Yearbook{}, err
	}
	defer func() { _ = tx.Rollback() }()
	y, err := scanBook(tx.QueryRowContext(ctx, `SELECT `+bookCols+` FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, publicID))
	if errors.Is(err, sql.ErrNoRows) {
		return Yearbook{}, ErrNotFound
	}
	if err != nil {
		return Yearbook{}, err
	}
	books := []Yearbook{y}
	if err := fill(ctx, tx, books, true); err != nil {
		return Yearbook{}, err
	}
	y = books[0]
	if err := change(&y); err != nil {
		return Yearbook{}, err
	}
	y.UpdatedAt = nowMs
	// A media id must belong to this very book; anything else (also another user's) is invalid_media.
	cover, err := mediaRow(ctx, tx, y.internalID, y.CoverMediaID)
	if err != nil {
		return Yearbook{}, err
	}
	photo, err := mediaRow(ctx, tx, y.internalID, y.Profile.PhotoMediaID)
	if err != nil {
		return Yearbook{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE yearbook_tab SET title=?, school_name=?, class_name=?, graduation_year=?, motto=?, language=?, page_size=?, cover_media_id=?, updated_at=? WHERE id = ?`,
		y.Title, y.SchoolName, y.ClassName, y.GraduationYear, y.Motto, y.Language, y.PageSize, cover, y.UpdatedAt, y.internalID); err != nil {
		return Yearbook{}, err
	}
	p := y.Profile
	if _, err := tx.ExecContext(ctx,
		`UPDATE profile_tab SET full_name=?, nickname=?, birthday=?, quote=?, hobbies=?, future_plans=?, photo_media_id=?, updated_at=? WHERE yearbook_id = ? AND is_owner = 1`,
		p.FullName, p.Nickname, p.Birthday, p.Quote, p.Hobbies, p.FuturePlans, photo, y.UpdatedAt, y.internalID); err != nil {
		return Yearbook{}, err
	}
	return y, tx.Commit()
}

// mediaRow resolves a public media id to its row id and checks that the photo belongs to the book (there is no
// foreign key to do it); nil stays NULL. The row is share-locked, so a concurrent delete of the photo (which clears
// the references first) waits for this transaction instead of leaving a dangling reference.
func mediaRow(ctx context.Context, tx *sql.Tx, bookID uint64, publicID *string) (sql.NullInt64, error) {
	if publicID == nil {
		return sql.NullInt64{}, nil
	}
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM media WHERE yearbook_id = ? AND public_id = ? FOR SHARE`, bookID, *publicID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, ValidationError{"invalid_media"}
	}
	return sql.NullInt64{Int64: id, Valid: err == nil}, err
}

// lockBook takes the owner's book row FOR UPDATE: the first lock of every transaction that changes a book or its children
// (lock order in modify). It reaches the row the same way modify does (owner_id, public_id), because locking through
// another index (the primary key first) takes the index records in the opposite order and deadlocks too.
// ErrNotFound when the book is gone or not the owner's.
func lockBook(ctx context.Context, tx *sql.Tx, ownerID uint64, publicID string) error {
	var id uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, publicID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ClearMediaRefs sets every cover and profile photo that points at the media row to NULL, in the caller's transaction.
// The media service calls it before it deletes the photo (this replaces ON DELETE SET NULL).
//
// Lock order: it first locks the photo's yearbook_tab row FOR UPDATE (as modify and Service.Delete do), then updates
// yearbook_tab and profile_tab. Without it a photo delete racing with setting that photo as cover deadlocked.
func (s *Store) ClearMediaRefs(ctx context.Context, tx *sql.Tx, mediaID int64) error {
	var ownerID uint64
	var publicID string
	err := tx.QueryRowContext(ctx, `SELECT owner_id, public_id FROM yearbook_tab WHERE id = (SELECT yearbook_id FROM media WHERE id = ?)`, mediaID).Scan(&ownerID, &publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // photo or book already gone, nothing refers to it
	}
	if err != nil {
		return err
	}
	if err := lockBook(ctx, tx, ownerID, publicID); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE yearbook_tab SET cover_media_id = NULL WHERE cover_media_id = ?`, mediaID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE profile_tab SET photo_media_id = NULL WHERE photo_media_id = ?`, mediaID)
	return err
}

func (s *Store) begin(ctx context.Context) (*sql.Tx, error) { return s.db.BeginTx(ctx, nil) }

func deleteProfiles(ctx context.Context, tx *sql.Tx, bookID uint64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM profile_tab WHERE yearbook_id = ?`, bookID)
	return err
}

// deleteBook removes the book row; ErrNotFound when it is gone already or not the owner's.
func deleteBook(ctx context.Context, tx *sql.Tx, ownerID, bookID uint64) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM yearbook_tab WHERE id = ? AND owner_id = ?`, bookID, ownerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
