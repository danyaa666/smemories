package booklayout

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/templates"
)

const (
	// DefaultSavesPerMinute is the default of SMEM_RATE_LAYOUT_SAVE_PER_MINUTE; Burst saves are allowed in any burstWindow.
	DefaultSavesPerMinute = 60
	Burst                 = 20
	burstWindow           = 10 * time.Second

	// SnapshotGap is how old the newest snapshot of a book must be before a save stores a new automatic one (T-163): saves inside the
	// gap are one editing burst. MaxSnapshots is how many snapshots a book keeps, newest first.
	SnapshotGap  = 10 * time.Minute
	MaxSnapshots = 30
)

// Codes of the save endpoint.
var (
	// CodeVersionConflict is the 409: base_version is not the stored version. The answer carries the stored document.
	CodeVersionConflict = apperr.NewCode("ERROR_VERSION_CONFLICT", apperr.Conflict.HTTP)
	// CodeInvalidLayout is the 400 for a rejected document. The answer carries the problem list.
	CodeInvalidLayout = apperr.NewCode("ERROR_INVALID_LAYOUT", apperr.Param.HTTP)
	// CodeVersionTemplateChanged is the 409 of a restore whose snapshot was made for another template than the book has now.
	CodeVersionTemplateChanged = apperr.NewCode("ERROR_VERSION_TEMPLATE_CHANGED", apperr.Conflict.HTTP)
)

// Results of a restore, the label of smem_layout_restores_total.
const (
	RestoreOK              = "ok"
	RestoreConflict        = "conflict"
	RestoreInvalid         = "invalid"
	RestoreTemplateChanged = "template_changed"
)

// Observer receives what the history does, for metrics and one log line per restore. Every member may be nil.
type Observer struct {
	Snapshot func(kind string) // a snapshot was stored (KindAuto, KindReset, KindRestore)
	Pruned   func(n int)       // old snapshots were deleted
	Restore  func(result string)
	Log      *slog.Logger // one line per restore: the book's public id, the version numbers and dropped; never document content
}

// Results of a save, the label of smem_layout_saves_total.
const (
	ResultOK          = "ok"
	ResultConflict    = "conflict"
	ResultInvalid     = "invalid"
	ResultTooLarge    = "too_large"
	ResultRateLimited = "rate_limited"
)

// RetryAfterError is the 429: the handler turns After into the Retry-After header.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// InvalidError is a rejected document with its problems (at most MaxProblems).
type InvalidError struct{ Problems []Problem }

func (e *InvalidError) Error() string { return "invalid layout" }

// ConflictError is a save with a stale base_version; Layout and Version are the stored ones (null and the current version when the row is gone).
type ConflictError struct {
	Version int64
	Layout  json.RawMessage
}

func (e *ConflictError) Error() string { return "layout version conflict" }

// DesignImages is the part of the media service the layout needs: which of the ids are design images of the owner's book.
type DesignImages interface {
	DesignImages(ctx context.Context, ownerID uint64, yearbookID string, ids []string) (map[string]bool, error)
}

// View is the answer of get.
type View struct {
	Layout     json.RawMessage `json:"layout"`      // the stored document; null when there is none
	Version    int64           `json:"version"`     // never decreases; 0 until the first save; after a reset or template change the next base_version
	TemplateID *string         `json:"template_id"` // the book's template; null when it has none
	UpdatedAt  *int64          `json:"updated_at"`  // Unix ms; null without a layout
}

// Saved is the answer of save.
type Saved struct {
	Version   int64 `json:"version"`
	UpdatedAt int64 `json:"updated_at"`
}

// Resolved is what the print route gets: the stored document re-validated (parts that no longer validate left out) and its version.
type Resolved struct {
	Layout  json.RawMessage
	Version int64
}

// Service holds the rules: owner scoping, validation, optimistic concurrency and the save rate limit.
type Service struct {
	store  *Store
	media  DesignImages
	minute ratelimit.Limiter // nil = unlimited (tests, no Redis)
	burst  ratelimit.Limiter
	now    func() time.Time
	count  func(result string) // smem_layout_saves_total; nil = not counted
	obs    Observer
}

// NewService builds the service; limiters and now may be nil (no rate limit, time.Now). perMinute is
// SMEM_RATE_LAYOUT_SAVE_PER_MINUTE.
func NewService(store *Store, media DesignImages, limiters *ratelimit.Factory, perMinute int, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	s := &Service{store: store, media: media, now: now}
	if limiters != nil {
		s.minute = limiters.Open("book_layout", perMinute, time.Minute) // fail open: it bounds effort, not guessing
		s.burst = limiters.Open("book_layout_burst", Burst, burstWindow)
	}
	return s
}

// SetCounter makes the service report every save's result (ResultOK, ...) to f.
func (s *Service) SetCounter(f func(result string)) { s.count = f }

// SetObserver makes the service report snapshots, pruning and restores to o.
func (s *Service) SetObserver(o Observer) { s.obs = o }

// Counted lets the handler report the one result the service never sees: a body over the size limit.
func (s *Service) Counted(result string) {
	if s.count != nil {
		s.count(result)
	}
}

// Get returns the owner's layout of the book.
func (s *Service) Get(ctx context.Context, ownerID uint64, publicID string) (*View, error) {
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	v := &View{TemplateID: st.templateID, Version: st.version}
	if st.row != nil {
		v.Layout, v.UpdatedAt = st.row.layout, &st.row.updatedAt
	}
	return v, nil
}

// Save validates layout against the book's current template and stores it if baseVersion is the stored version (0 = none yet).
// Another user's and an unknown book are ERROR_NOT_FOUND, also for an invalid document. A rejected document is an *InvalidError, a stale
// baseVersion a *ConflictError, a template without a layout block an *InvalidError too.
func (s *Service) Save(ctx context.Context, ownerID uint64, publicID string, baseVersion int64, layout []byte) (*Saved, error) {
	res, err := s.save(ctx, ownerID, publicID, baseVersion, layout)
	switch {
	case err == nil:
		s.Counted(ResultOK)
	case apperr.Is(err, apperr.RateLimited):
		s.Counted(ResultRateLimited)
	case apperr.Is(err, CodeVersionConflict):
		s.Counted(ResultConflict)
	case apperr.Is(err, CodeInvalidLayout):
		s.Counted(ResultInvalid)
	}
	return res, err
}

func (s *Service) save(ctx context.Context, ownerID uint64, publicID string, baseVersion int64, layout []byte) (*Saved, error) {
	if baseVersion < 0 {
		return nil, apperr.New(apperr.Param, "base_version must be 0 or the version read")
	}
	if err := s.take(ctx, ownerID); err != nil {
		return nil, err
	}
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	block := layoutOf(st.templateID)
	if block == nil {
		return nil, invalid(Problem{"", "template_not_customisable"})
	}
	own, err := s.ownImages(ctx, ownerID, publicID, layout)
	if err != nil {
		return nil, err
	}
	doc, probs := Validate(layout, block, own)
	if len(probs) > 0 {
		return nil, invalid(probs...)
	}
	pol := s.policy()
	w, lost, err := s.store.save(ctx, ownerID, publicID, *st.templateID, baseVersion, doc, KindAuto, pol)
	if err != nil {
		return nil, err
	}
	if lost != nil {
		return nil, conflictOf(lost)
	}
	s.written(w)
	return &Saved{Version: w.version, UpdatedAt: pol.nowMs}, nil
}

func (s *Service) policy() policy {
	return policy{nowMs: s.now().UnixMilli(), gapMs: SnapshotGap.Milliseconds(), keep: MaxSnapshots}
}

// written reports the snapshot a committed write stored and the ones it pruned.
func (s *Service) written(w *written) {
	if w.snap != "" && s.obs.Snapshot != nil {
		s.obs.Snapshot(w.snap)
	}
	if w.pruned > 0 && s.obs.Pruned != nil {
		s.obs.Pruned(int(w.pruned))
	}
}

// ownImages asks the media service which of the media ids the document names are design images of the owner's book: the one check that
// a media id is not a profile photo, a note photo or another user's image.
func (s *Service) ownImages(ctx context.Context, ownerID uint64, publicID string, doc []byte) (map[string]bool, error) {
	ids := MediaRefs(doc)
	if len(ids) == 0 {
		return nil, nil
	}
	return s.media.DesignImages(ctx, ownerID, publicID, ids)
}

// Reset deletes the layout (back to the template's own pages); it succeeds when there is none.
// The deleted document is kept as a snapshot (T-163).
func (s *Service) Reset(ctx context.Context, ownerID uint64, publicID string) error {
	w, err := s.store.reset(ctx, ownerID, publicID, s.policy())
	if err != nil {
		return err
	}
	s.written(w)
	return nil
}

// Resolved is the layout to print for the owner's book (book-data/get): nil when there is none, the row was made for another template, the
// template has no layout block or nothing of it validates. templateID is the book's template as the caller read it.
func (s *Service) Resolved(ctx context.Context, ownerID uint64, publicID string, templateID *string) (*Resolved, error) {
	block := layoutOf(templateID)
	if block == nil {
		return nil, nil
	}
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	if st.row == nil || st.row.templateID != *templateID {
		return nil, nil
	}
	own, err := s.ownImages(ctx, ownerID, publicID, st.row.layout)
	if err != nil {
		return nil, err
	}
	doc := Sanitize(st.row.layout, block, own)
	if doc == nil {
		return nil, nil
	}
	return &Resolved{Layout: doc, Version: st.row.version}, nil
}

// DeleteByYearbook removes the book's layout row and its snapshots in the caller's transaction. It is the yearbook.ChildPurger of a book
// delete and the hook of a template change (a layout, and so its history, is only valid for the template it was made for).
func (s *Service) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	if err := deleteRow(ctx, tx, yearbookID); err != nil {
		return err
	}
	return deleteVersions(ctx, tx, yearbookID)
}

// take spends one save: Burst per burstWindow and the configured number per minute. Both fail open when Redis is down.
func (s *Service) take(ctx context.Context, ownerID uint64) error {
	if s.minute == nil {
		return nil
	}
	key := strconv.FormatUint(ownerID, 10)
	if ok, wait, _ := s.burst.Take(ctx, key); !ok {
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many saves, try again later"), After: wait}
	}
	if ok, wait, _ := s.minute.Take(ctx, key); !ok {
		_ = s.burst.Refund(ctx, key) // a refused save is not a burst hit
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many saves, try again later"), After: wait}
	}
	return nil
}

func layoutOf(templateID *string) *templates.LayoutBlock {
	if templateID == nil {
		return nil
	}
	d, ok := templates.Design(*templateID)
	if !ok {
		return nil
	}
	return d.Layout
}

func invalid(p ...Problem) error {
	return apperr.Wrap(CodeInvalidLayout, &InvalidError{Problems: p}, "layout is not valid")
}

func conflictOf(l *conflict) error {
	c := &ConflictError{Version: l.version}
	if l.row != nil {
		c.Layout = l.row.layout
	}
	return apperr.Wrap(CodeVersionConflict, c, "the layout was saved elsewhere")
}

// Versions lists the owner's snapshots of the book, newest first (at most MaxSnapshots), without documents.
func (s *Service) Versions(ctx context.Context, ownerID uint64, publicID string) ([]VersionInfo, error) {
	if err := s.take(ctx, ownerID); err != nil {
		return nil, err
	}
	return s.store.versions(ctx, ownerID, publicID, MaxSnapshots)
}

// VersionView is the answer of version-get.
type VersionView struct {
	ID        string          `json:"id"`
	Version   int64           `json:"version"`
	Kind      string          `json:"kind"`
	CreatedAt int64           `json:"created_at"`
	Layout    json.RawMessage `json:"layout"`
}

// Version returns one snapshot with its document. Another user's book, an unknown book and an unknown snapshot are ERROR_NOT_FOUND.
func (s *Service) Version(ctx context.Context, ownerID uint64, publicID, id string) (*VersionView, error) {
	if err := s.take(ctx, ownerID); err != nil {
		return nil, err
	}
	v, err := s.store.version(ctx, ownerID, publicID, id)
	if err != nil {
		return nil, err
	}
	return &VersionView{ID: v.ID, Version: v.Version, Kind: v.Kind, CreatedAt: v.CreatedAt, Layout: v.layout}, nil
}

// Restored is the answer of restore.
type Restored struct {
	Version   int64           `json:"version"`
	UpdatedAt int64           `json:"updated_at"`
	Layout    json.RawMessage `json:"layout"`
	Dropped   int             `json:"dropped"`
}

// Restore writes snapshot id as a new version of the book if baseVersion is the current one. The document is validated like a save
// against the book's current template and the owner's current design images; items and backgrounds whose asset or image is gone are
// dropped and counted. The replaced document becomes a restore snapshot, in the same transaction. A snapshot made for another template is
// ERROR_VERSION_TEMPLATE_CHANGED, a stale baseVersion ERROR_VERSION_CONFLICT with the stored document.
func (s *Service) Restore(ctx context.Context, ownerID uint64, publicID, id string, baseVersion int64) (*Restored, error) {
	res, err := s.restore(ctx, ownerID, publicID, id, baseVersion)
	result := ""
	switch {
	case err == nil:
		result = RestoreOK
	case apperr.Is(err, CodeVersionConflict):
		result = RestoreConflict
	case apperr.Is(err, CodeInvalidLayout):
		result = RestoreInvalid
	case apperr.Is(err, CodeVersionTemplateChanged), apperr.Is(err, apperr.Conflict): // the second: the template changed while we validated
		result = RestoreTemplateChanged
	}
	if result != "" && s.obs.Restore != nil {
		s.obs.Restore(result)
	}
	if err == nil && s.obs.Log != nil {
		s.obs.Log.InfoContext(ctx, "layout restored", "yearbook_id", publicID, "restored_version", res.From, "from_version", res.Replaced, "new_version", res.Version, "dropped", res.Dropped)
	}
	if err != nil {
		return nil, err
	}
	return &res.Restored, nil
}

type restoreResult struct {
	Restored
	From, Replaced int64 // the version the snapshot held, the version the restore replaced
}

func (s *Service) restore(ctx context.Context, ownerID uint64, publicID, id string, baseVersion int64) (*restoreResult, error) {
	if baseVersion < 0 {
		return nil, apperr.New(apperr.Param, "base_version must be 0 or the version read")
	}
	if err := s.take(ctx, ownerID); err != nil {
		return nil, err
	}
	snap, err := s.store.version(ctx, ownerID, publicID, id)
	if err != nil {
		return nil, err
	}
	block := layoutOf(snap.bookTpl)
	if block == nil {
		return nil, invalid(Problem{"", "template_not_customisable"})
	}
	if snap.templateID != *snap.bookTpl {
		return nil, apperr.New(CodeVersionTemplateChanged, "the layout was made for another template")
	}
	own, err := s.ownImages(ctx, ownerID, publicID, snap.layout)
	if err != nil {
		return nil, err
	}
	doc, dropped, probs := ValidateRestore(snap.layout, block, own)
	if len(probs) > 0 {
		return nil, invalid(probs...)
	}
	pol := s.policy()
	w, lost, err := s.store.save(ctx, ownerID, publicID, *snap.bookTpl, baseVersion, doc, KindRestore, pol)
	if err != nil {
		return nil, err
	}
	if lost != nil {
		return nil, conflictOf(lost)
	}
	s.written(w)
	return &restoreResult{Restored: Restored{Version: w.version, UpdatedAt: pol.nowMs, Layout: doc, Dropped: dropped}, From: snap.Version, Replaced: baseVersion}, nil
}
