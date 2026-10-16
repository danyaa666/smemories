package notes

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notefields"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/textx"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	maxLabel = 60

	// Public routes. A class behind one campus address opens the same link, so only requests that miss (unknown,
	// malformed or revoked token) count against the tight per-IP limit; the generous one is a cost guard.
	missesPerWindow = 60
	allPerWindow    = 600
	publicWindow    = 15 * time.Minute

	// Submission limits count requests that passed text validation. They are sized for a class behind one
	// campus address (40 students each submit once); the per-collection limit and the 300-note cap are the real brakes.
	submitPerIPHour         = 100
	submitPerIPDay          = 300
	submitPerCollectionHour = 60

	maxDeadlineYears = 1
	mediaBusyRetry   = 2 * time.Second
)

// plainID matches the ids worth echoing back in an error; an unknown id chosen by the client is not echoed otherwise.
var plainID = regexp.MustCompile(`^[a-z_]{1,40}$`)

// store is what the service needs from its adapter (declared here, on the consumer side).
type store interface {
	create(ctx context.Context, ownerID uint64, bookID string, c *Collection, hash [32]byte) error
	list(ctx context.Context, ownerID uint64, bookID string) ([]*Collection, error)
	revoke(ctx context.Context, ownerID uint64, id string, now int64) error
	lookup(ctx context.Context, hash [32]byte) (*Public, error)
	noteCount(ctx context.Context, collectionID uint64) (int, error)
	insertNote(ctx context.Context, n *newNote) error
}

// photos is the part of the media service that submissions use.
type photos interface {
	UploadContributorFrom(ctx context.Context, yearbookRow uint64, load func() ([]byte, error)) (media.Media, error)
	Discard(ms ...media.Media) error
}

// Service holds the rules of the notes feature: who may create and revoke links, what a visitor with a link may do, and what
// became of the foreign keys of the notes tables (docs/db-conventions.md, "Deleting"): the yearbook service calls it to delete a
// book's collections, notes and photo links (yearbook.ChildPurger), and the media service calls it before a photo row is deleted
// (media.RefClearer). It does not know about HTTP.
//
// The limiters fail open when Redis is down: they bound load and cost, not guessing.
type Service struct {
	store                      store
	media                      photos
	logger                     *slog.Logger
	now                        func() time.Time
	misses, all                ratelimit.Limiter // public routes, per client IP
	subIPHr, subIPDay, subColl ratelimit.Limiter // submissions per IP per hour and day, per collection per hour

	mod        modStore          // moderation (moderation.go)
	purge      notePhotos        // photos of deleted notes
	tokens     Tokens            // next_id tokens of the note list
	moderation ratelimit.Limiter // moderation writes per user
}

// NewService builds the service; now may be nil (time.Now). mediaSvc stores the photos of submissions; tokens seals the
// next_id of the owner's note list.
func NewService(st *Store, mediaSvc *media.Service, tokens Tokens, limiters *ratelimit.Factory, logger *slog.Logger, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, media: mediaSvc, logger: logger, now: now,
		misses: limiters.Open("public_misses", missesPerWindow, publicWindow), all: limiters.Open("public_all", allPerWindow, publicWindow),
		subIPHr: limiters.Open("submit_ip_hour", submitPerIPHour, time.Hour), subIPDay: limiters.Open("submit_ip_day", submitPerIPDay, 24*time.Hour),
		subColl: limiters.Open("submit_collection_hour", submitPerCollectionHour, time.Hour),
		mod:     st, purge: mediaSvc, tokens: tokens, moderation: limiters.Open("note_moderation", moderationPerMinute, time.Minute)}
}

// CreateInput is a request for a new link. DeadlineAt is Unix ms.
type CreateInput struct {
	YearbookID string
	Label      *string
	DeadlineAt *int64
}

// Created is a new link; Token is the bearer secret, shown here and nowhere else.
type Created struct {
	*Collection
	Token string
}

// CreateCollection makes a link for a book of the owner: the owner's email must be verified, at most maxActive links stay active.
func (s *Service) CreateCollection(ctx context.Context, ownerID uint64, emailVerified bool, in *CreateInput) (*Created, error) {
	if !emailVerified {
		return nil, apperr.New(CodeEmailNotVerified, "verify your email first")
	}
	if in.YearbookID == "" {
		return nil, apperr.New(apperr.Param, "yearbook_id is required")
	}
	now := s.now().UTC()
	c := &Collection{ID: ulid.New(now), CreatedAt: now.UnixMilli()}
	if in.Label != nil {
		l, ok := textx.Clean(*in.Label, 0, maxLabel)
		if !ok {
			return nil, apperr.New(CodeInvalidLabel, "label must be at most "+strconv.Itoa(maxLabel)+" characters without control characters")
		}
		c.Label = l
	}
	if in.DeadlineAt != nil {
		d := *in.DeadlineAt
		if d <= c.CreatedAt || d > now.AddDate(maxDeadlineYears, 0, 0).UnixMilli() {
			return nil, apperr.New(CodeInvalidDeadline, "deadline_at must be in the future and at most one year ahead")
		}
		c.DeadlineAt = in.DeadlineAt
	}
	token, hash, err := newToken()
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "generate link token")
	}
	if err := s.store.create(ctx, ownerID, in.YearbookID, c, hash); err != nil {
		return nil, err
	}
	return &Created{Collection: c, Token: token}, nil
}

// ListCollections returns the book's links, newest first, revoked ones included. ponytail: unpaged (at most five are active); add a cursor if the revoked ones pile up.
func (s *Service) ListCollections(ctx context.Context, ownerID uint64, yearbookID string) ([]*Collection, error) {
	if yearbookID == "" {
		return nil, apperr.New(apperr.Param, "yearbook_id is required")
	}
	return s.store.list(ctx, ownerID, yearbookID)
}

// RevokeCollection stops a link at once; revoking it again succeeds.
func (s *Service) RevokeCollection(ctx context.Context, ownerID uint64, id string) error {
	if id == "" {
		return apperr.New(apperr.Param, "id is required")
	}
	return s.store.revoke(ctx, ownerID, id, s.now().UnixMilli())
}

// Resolve is the front of both public routes: the per-IP limits, then the token. The link must be active and before its
// deadline. Unknown, malformed and revoked tokens are all ErrNotFound; the limiters answer a *RetryAfterError.
func (s *Service) Resolve(ctx context.Context, ip, token string) (*Public, error) {
	if ok, wait, _ := s.all.Take(ctx, ip); !ok {
		return nil, tooMany(wait)
	}
	if ok, wait, _ := s.misses.Take(ctx, ip); !ok {
		return nil, tooMany(wait)
	}
	var p *Public
	hash, valid := tokenHash(token)
	err := ErrNotFound
	if valid {
		p, err = s.store.lookup(ctx, hash)
	}
	if !apperr.Is(err, apperr.NotFound) {
		_ = s.misses.Refund(ctx, ip) // a valid token is not a guess
	}
	if err != nil {
		return nil, err
	}
	if p.DeadlineAt != nil && s.now().UnixMilli() >= *p.DeadlineAt {
		return nil, ErrClosed
	}
	return p, nil
}

// Lookup is Resolve plus the questions of the note form.
func (s *Service) Lookup(ctx context.Context, ip, token string) (*Public, []notefields.FieldInfo, error) {
	p, err := s.Resolve(ctx, ip, token)
	if err != nil {
		return nil, nil, err
	}
	fields, err := notefields.Info(FieldsFor(p.TemplateID))
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.Internal, err, "describe note fields")
	}
	return p, fields, nil
}

// Submission is a friend's note as the transport read it. Photos hold one loader per photo, in order; a loader is called only
// when the media service has a free slot, so the bytes of one photo at a time are in memory. Verify is the abuse check (nil =
// none); it runs after the text was validated and before any photo is processed.
type Submission struct {
	IP      string
	Answers []byte
	Photos  []func() ([]byte, error)
	Verify  func(ctx context.Context) error
}

// CheckOpen reports ErrFull when the link p already holds maxNotes notes. The handler calls it right after Resolve,
// before the body is read, so a full link costs no disk (T-034); SubmitNote checks again for the race.
func (s *Service) CheckOpen(ctx context.Context, p *Public) error {
	if n, err := s.store.noteCount(ctx, p.CollectionID); err != nil || n >= maxNotes {
		return cmp.Or(err, ErrFull)
	}
	return nil
}

// SubmitNote stores a note as pending in the link p (from Resolve). Order matters: the note limit and the text are checked
// before any photo is decoded, the rate limits count only requests that passed text validation, and every failure after a
// photo was stored deletes what was stored. It returns the id of the note.
func (s *Service) SubmitNote(ctx context.Context, p *Public, in *Submission) (string, error) {
	if n, err := s.store.noteCount(ctx, p.CollectionID); err != nil || n >= maxNotes {
		return "", cmp.Or(err, ErrFull)
	}
	now := s.now().UTC()
	clean, err := validateAnswers(p, in.Answers)
	if err != nil {
		return "", err
	}
	if in.Verify != nil {
		if err := in.Verify(ctx); err != nil {
			return "", apperr.Wrap(CodeVerificationFailed, err, "verification failed")
		}
	}
	coll := strconv.FormatUint(p.CollectionID, 10)
	if ok, wait := s.takeSubmit(ctx, in.IP, coll); !ok {
		return "", tooMany(wait)
	}

	var stored []media.Media
	fail := func(err error) error {
		if errors.Is(err, media.ErrBusy) { // turned away, not misused: give back what the request cost
			_ = s.all.Refund(ctx, in.IP)
			_ = s.subIPHr.Refund(ctx, in.IP)
			_ = s.subIPDay.Refund(ctx, in.IP)
			_ = s.subColl.Refund(ctx, coll)
		}
		if derr := s.media.Discard(stored...); derr != nil {
			s.logger.Error("notes: could not remove the photos of a failed submission", "error", derr)
		}
		return mediaError(err)
	}
	rows := make([]int64, 0, len(in.Photos))
	for _, load := range in.Photos { // one photo at a time; the media service bounds decoding and memory
		m, err := s.media.UploadContributorFrom(ctx, p.YearbookID, load)
		if err != nil {
			return "", fail(err)
		}
		stored = append(stored, m)
		rows = append(rows, m.RowID)
	}
	answers, err := json.Marshal(clean)
	if err != nil {
		return "", fail(apperr.Wrap(apperr.Internal, err, "encode answers"))
	}
	id := ulid.New(now)
	if err := s.store.insertNote(ctx, &newNote{ID: id, Answers: answers, MediaRows: rows, CreatedAt: now.UnixMilli(), Collection: p.CollectionID, Yearbook: p.YearbookID}); err != nil {
		return "", fail(err)
	}
	return id, nil
}

// validateAnswers parses the answers part and checks it against the form's fields. It returns the cleaned answers.
func validateAnswers(p *Public, raw []byte) (map[string]string, error) {
	var answers map[string]string
	if b := bytes.TrimSpace(raw); !utf8.Valid(b) || len(b) == 0 || b[0] != '{' || json.Unmarshal(b, &answers) != nil {
		return nil, apperr.New(apperr.Param, "answers must be a JSON object of strings")
	}
	clean, err := notefields.Validate(FieldsFor(p.TemplateID), answers)
	var fe *notefields.FieldError
	if errors.As(err, &fe) {
		msg := fe.Code // never the value; the id only when it is one of ours or plainly harmless
		if fe.Code != notefields.CodeUnknownField || plainID.MatchString(fe.ID) {
			msg += ": " + fe.ID
		}
		switch fe.Code {
		case notefields.CodeMissingAnswer:
			return nil, apperr.New(CodeMissingAnswer, msg)
		case notefields.CodeInvalidAnswer:
			return nil, apperr.New(CodeInvalidAnswer, msg)
		}
		return nil, apperr.New(apperr.Param, msg) // an unknown field
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "validate answers")
	}
	return clean, nil
}

// takeSubmit counts a submission against the per-IP and per-collection limits; none is counted unless all allow it.
func (s *Service) takeSubmit(ctx context.Context, ip, collection string) (bool, time.Duration) {
	if ok, wait, _ := s.subIPHr.Take(ctx, ip); !ok {
		return false, wait
	}
	if ok, wait, _ := s.subIPDay.Take(ctx, ip); !ok {
		_ = s.subIPHr.Refund(ctx, ip)
		return false, wait
	}
	if ok, wait, _ := s.subColl.Take(ctx, collection); !ok {
		_ = s.subIPHr.Refund(ctx, ip)
		_ = s.subIPDay.Refund(ctx, ip)
		return false, wait
	}
	return true, 0
}

// mediaError gives the errors of the photo pipeline their codes; any other error is returned as it is.
func mediaError(err error) error {
	switch {
	case errors.Is(err, media.ErrUnsupported):
		return apperr.Wrap(CodeUnsupportedMediaType, err, "only JPEG, PNG and WebP images are accepted")
	case errors.Is(err, media.ErrInvalidImage):
		return apperr.Wrap(CodeInvalidImage, err, "the image is corrupt or too large")
	case errors.Is(err, media.ErrQuota):
		return apperr.Wrap(CodeQuotaExceeded, err, "storage quota reached")
	case errors.Is(err, media.ErrBusy):
		return &RetryAfterError{Err: apperr.Wrap(CodeBusy, err, "too many uploads in progress, retry shortly"), After: mediaBusyRetry}
	case apperr.Is(err, media.CodeStorage):
		return apperr.Wrap(CodeStorage, err, "object storage failed")
	case errors.Is(err, media.ErrNotFound):
		return ErrNotFound
	}
	return err
}

// DeleteByYearbook deletes the collections, notes and photo links of a yearbook in the caller's transaction, which holds the
// yearbook row lock (yearbook.Service.Delete). Collections and notes go together; revoking a collection keeps its notes.
func (s *Service) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	return deleteByYearbook(ctx, tx, yearbookID)
}

// ClearMediaRefs deletes the note links of a photo, in the caller's transaction, just before the photo row is deleted. The media
// service calls it after the yearbook's own ClearMediaRefs, so the yearbook row is already locked.
func (s *Service) ClearMediaRefs(ctx context.Context, tx *sql.Tx, mediaID int64) error {
	return deleteMediaLinks(ctx, tx, mediaID)
}

// compile-time: the real adapter satisfies the consumer-side interface.
var _ store = (*Store)(nil)
