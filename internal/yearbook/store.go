package yearbook

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	// ErrNotFound covers a book that does not exist and one owned by someone else: callers cannot tell them apart.
	ErrNotFound     = errors.New("yearbook: not found")
	ErrLimitReached = errors.New("yearbook: book limit reached")
)

// Yearbook is the book as the API shows it; internalID is never serialised (L-05).
type Yearbook struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	SchoolName     string    `json:"school_name"`
	ClassName      string    `json:"class_name"`
	GraduationYear *int      `json:"graduation_year"`
	Motto          string    `json:"motto"`
	Language       string    `json:"language"`
	PageSize       string    `json:"page_size"`
	TemplateID     *string   `json:"template_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Profile        Profile   `json:"profile"`
	internalID     int64
}

// Profile is the owner's profile page information.
type Profile struct {
	ID          string  `json:"id"`
	IsOwner     bool    `json:"is_owner"`
	FullName    string  `json:"full_name"`
	Nickname    string  `json:"nickname"`
	Birthday    *string `json:"birthday"` // YYYY-MM-DD
	Quote       string  `json:"quote"`
	Hobbies     string  `json:"hobbies"`
	FuturePlans string  `json:"future_plans"`
}

// Store persists yearbooks and profiles. Every query is parameterised and scoped by owner_id:
// a book is never fetched by id alone.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const bookCols = `y.id, y.public_id, y.title, y.school_name, y.class_name, y.graduation_year, y.motto, y.language, y.page_size, y.template_id,
 y.created_at, y.updated_at, p.public_id, p.is_owner, p.full_name, p.nickname, p.birthday, p.quote, p.hobbies, p.future_plans`

const bookFrom = ` FROM yearbooks y JOIN profiles p ON p.yearbook_id = y.id AND p.is_owner`

type scanner interface{ Scan(...any) error }

func scanBook(sc scanner) (Yearbook, error) {
	var y Yearbook
	var grad sql.Null[int]
	var tmpl sql.NullString
	var bday sql.NullTime
	p := &y.Profile
	err := sc.Scan(&y.internalID, &y.ID, &y.Title, &y.SchoolName, &y.ClassName, &grad, &y.Motto, &y.Language, &y.PageSize, &tmpl,
		&y.CreatedAt, &y.UpdatedAt, &p.ID, &p.IsOwner, &p.FullName, &p.Nickname, &bday, &p.Quote, &p.Hobbies, &p.FuturePlans)
	if err != nil {
		return Yearbook{}, err
	}
	if grad.Valid {
		y.GraduationYear = &grad.V
	}
	if tmpl.Valid {
		y.TemplateID = &tmpl.String
	}
	if bday.Valid {
		b := bday.Time.Format(dateLayout)
		p.Birthday = &b
	}
	y.CreatedAt, y.UpdatedAt = y.CreatedAt.UTC(), y.UpdatedAt.UTC()
	return y, nil
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
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM yearbooks WHERE owner_id = ?`, ownerID).Scan(&n); err != nil {
		return err
	}
	if n >= maxBooksOwned {
		return ErrLimitReached
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO yearbooks (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		y.ID, ownerID, y.Title, y.SchoolName, y.ClassName, y.GraduationYear, y.Motto, y.Language, y.PageSize, y.CreatedAt, y.UpdatedAt)
	if err != nil {
		return err
	}
	bookID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if err := insertProfile(ctx, tx, bookID, y.Profile); err != nil {
		return err
	}
	return tx.Commit()
}

func insertProfile(ctx context.Context, tx *sql.Tx, bookID int64, p Profile) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO profiles (public_id, yearbook_id, is_owner, full_name, nickname, birthday, quote, hobbies, future_plans) VALUES (?,?,?,?,?,?,?,?,?)`,
		p.ID, bookID, true, p.FullName, p.Nickname, p.Birthday, p.Quote, p.Hobbies, p.FuturePlans)
	return err
}

func (s *Store) get(ctx context.Context, ownerID uint64, publicID string) (Yearbook, error) {
	y, err := scanBook(s.db.QueryRowContext(ctx, `SELECT `+bookCols+bookFrom+` WHERE y.owner_id = ? AND y.public_id = ?`, ownerID, publicID))
	if errors.Is(err, sql.ErrNoRows) {
		return Yearbook{}, ErrNotFound
	}
	return y, err
}

// cursor is the position after the last book of a page: books are ordered by (updated_at, public_id) descending.
type cursor struct {
	updatedAt time.Time
	publicID  string
}

// list returns up to limit books, newest updated_at first, starting after c (nil = first page).
func (s *Store) list(ctx context.Context, ownerID uint64, c *cursor, limit int) ([]Yearbook, error) {
	q, args := `SELECT `+bookCols+bookFrom+` WHERE y.owner_id = ?`, []any{ownerID}
	if c != nil {
		q += ` AND (y.updated_at < ? OR (y.updated_at = ? AND y.public_id < ?))`
		args = append(args, c.updatedAt, c.updatedAt, c.publicID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY y.updated_at DESC, y.public_id DESC LIMIT ?`, append(args, limit)...)
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
	return out, rows.Err()
}

// modify locks the owner's book, lets change edit it (and write the profile through the same
// transaction when it wants to), bumps updated_at to now and returns the result.
func (s *Store) modify(ctx context.Context, ownerID uint64, publicID string, now time.Time, change func(*Yearbook) error) (Yearbook, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Yearbook{}, err
	}
	defer func() { _ = tx.Rollback() }()
	y, err := scanBook(tx.QueryRowContext(ctx, `SELECT `+bookCols+bookFrom+` WHERE y.owner_id = ? AND y.public_id = ? FOR UPDATE`, ownerID, publicID))
	if errors.Is(err, sql.ErrNoRows) {
		return Yearbook{}, ErrNotFound
	}
	if err != nil {
		return Yearbook{}, err
	}
	if err := change(&y); err != nil {
		return Yearbook{}, err
	}
	y.UpdatedAt = now.UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE yearbooks SET title=?, school_name=?, class_name=?, graduation_year=?, motto=?, language=?, page_size=?, updated_at=? WHERE id = ?`,
		y.Title, y.SchoolName, y.ClassName, y.GraduationYear, y.Motto, y.Language, y.PageSize, y.UpdatedAt, y.internalID); err != nil {
		return Yearbook{}, err
	}
	p := y.Profile
	if _, err := tx.ExecContext(ctx,
		`UPDATE profiles SET full_name=?, nickname=?, birthday=?, quote=?, hobbies=?, future_plans=? WHERE yearbook_id = ? AND is_owner`,
		p.FullName, p.Nickname, p.Birthday, p.Quote, p.Hobbies, p.FuturePlans, y.internalID); err != nil {
		return Yearbook{}, err
	}
	return y, tx.Commit()
}

func (s *Store) delete(ctx context.Context, ownerID uint64, publicID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM yearbooks WHERE owner_id = ? AND public_id = ?`, ownerID, publicID)
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
