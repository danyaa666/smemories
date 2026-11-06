package booklayout

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/textx"
	"github.com/danyaa666/smemories/internal/ulid"
)

// Own page templates and the apply API (E09 T-173, ADR 0005 Q3). A student saves one page of their book as a template (book_template_tab,
// owner only) and applies a shipped or own template to a page of a book: pages.<kind> is replaced and the whole document re-validated in
// one write with one history snapshot, so a bad template or a conflict never leaves a half-applied page. Rules: docs/templates.md,
// "Own templates and applying a template".

// MaxOwnTemplates is how many own templates a user keeps; MaxTemplateBytes (pagetemplate.go) is the size of each.
const MaxOwnTemplates = 20

// Codes of the template routes.
var (
	// CodeTemplateLimit is the 409 of template-create when the user already has MaxOwnTemplates; data is {limit}.
	CodeTemplateLimit = apperr.NewCode("ERROR_TEMPLATE_LIMIT", apperr.Conflict.HTTP)
	// CodeTemplateMismatch is the 409 of apply-template for an own template made on another book template than the book has now.
	CodeTemplateMismatch = apperr.NewCode("ERROR_TEMPLATE_MISMATCH", apperr.Conflict.HTTP)
)

// Results of an apply, the label of smem_page_template_applies_total.
const (
	ApplyOK       = "ok"
	ApplyConflict = "conflict"
	ApplyInvalid  = "invalid"
	ApplyMismatch = "mismatch"
)

// LimitError is the 409 ERROR_TEMPLATE_LIMIT; the handler puts Limit into data.
type LimitError struct{ Limit int }

func (e *LimitError) Error() string { return "own template limit reached" }

// OwnTemplate is the answer of template-create: the new template without its page, and how many parts were left out of it.
type OwnTemplate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	TemplateID string `json:"template_id"`
	ItemCount  int    `json:"item_count"`
	Bytes      int    `json:"bytes"`
	CreatedAt  int64  `json:"created_at"`
	Stripped   int    `json:"stripped"` // image items and the picture background that were not copied
}

// strip removes what an own template must not carry: image items and a picture background (own images are not shareable). It returns the
// page and how many parts it removed. page is the value of pages.<kind> of a stored (validated) document.
func strip(page json.RawMessage) (json.RawMessage, int, error) {
	var p map[string]json.RawMessage
	if err := json.Unmarshal(page, &p); err != nil || p == nil {
		return nil, 0, apperr.New(apperr.Internal, "stored page is not an object")
	}
	n := 0
	if bg, ok := p["bg"]; ok {
		var b map[string]json.RawMessage
		if json.Unmarshal(bg, &b) == nil {
			if _, media := b["media"]; media {
				delete(p, "bg")
				n++
			}
		}
	}
	if raw, ok := p["items"]; ok {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, 0, apperr.New(apperr.Internal, "stored items are not a list")
		}
		kept := make([]json.RawMessage, 0, len(items))
		for _, it := range items {
			var t struct {
				T string `json:"t"`
			}
			_ = json.Unmarshal(it, &t)
			if t.T == "image" {
				n++
				continue
			}
			kept = append(kept, it)
		}
		b, _ := json.Marshal(kept)
		p["items"] = b
	}
	out, err := json.Marshal(p)
	return out, n, err
}

// pageDoc wraps one page as a one-page document, the form Validate reads.
func pageDoc(kind string, page []byte) []byte {
	k, _ := json.Marshal(kind)
	return []byte(`{"v":1,"pages":{` + string(k) + `:` + string(page) + `}}`)
}

// pageOf returns pages.<kind> of a document, nil when there is none. Keys are matched exactly (a struct field would match "pAgEs" too, which
// withPage would then write next to "pages": found by FuzzWithPage, not reachable with a validated document).
func pageOf(doc []byte, kind string) json.RawMessage {
	var root map[string]json.RawMessage
	var pages map[string]json.RawMessage
	if json.Unmarshal(doc, &root) != nil || json.Unmarshal(root["pages"], &pages) != nil {
		return nil
	}
	return pages[kind]
}

// withPage returns the document cur (nil = none yet) with pages.<kind> replaced by page and everything else as it was.
func withPage(cur []byte, kind string, page []byte) ([]byte, error) {
	root := map[string]json.RawMessage{"v": json.RawMessage("1")}
	pages := map[string]json.RawMessage{}
	if cur != nil {
		if err := json.Unmarshal(cur, &root); err != nil {
			return nil, apperr.Wrap(apperr.Internal, err, "stored layout")
		}
		if root == nil { // "null" unmarshals to a nil map (found by FuzzWithPage)
			return nil, apperr.New(apperr.Internal, "stored layout")
		}
		if raw, ok := root["pages"]; ok {
			if err := json.Unmarshal(raw, &pages); err != nil || pages == nil {
				return nil, apperr.New(apperr.Internal, "stored pages")
			}
		}
	}
	pages[kind] = page
	b, err := json.Marshal(pages)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "encode pages")
	}
	root["pages"] = b
	out, err := json.Marshal(root)
	return out, apperr.WithCode(apperr.Internal, err)
}

// CreateTemplate saves the book's current pages.<kind> as an own template called name. Image items and a picture background are not copied
// (Stripped counts them). Another user's and an unknown book are ERROR_NOT_FOUND; a book without that page saved is ERROR_PARAM; the
// MaxOwnTemplates+1st is ERROR_TEMPLATE_LIMIT; a page over MaxTemplateBytes is ERROR_TOO_LARGE; a page that no longer validates is
// ERROR_INVALID_LAYOUT.
func (s *Service) CreateTemplate(ctx context.Context, ownerID uint64, publicID, kind, name string) (*OwnTemplate, error) {
	if err := s.take(ctx, ownerID); err != nil {
		return nil, err
	}
	name, ok := textx.Clean(name, 1, MaxLabelRunes)
	if !ok {
		return nil, apperr.New(apperr.Param, "name must be plain text of 1 to 60 characters")
	}
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	block := layoutOf(st.templateID)
	if block == nil {
		return nil, invalid(Problem{"", "template_not_customisable"})
	}
	k, ok := block.Kind(kind)
	if !ok {
		return nil, apperr.New(apperr.Param, "kind must be a page kind of the book's template")
	}
	var page json.RawMessage
	if st.row != nil {
		page = pageOf(st.row.layout, k.ID)
	}
	if page == nil {
		return nil, apperr.New(apperr.Param, "the book has no saved page of this kind")
	}
	page, stripped, err := strip(page)
	if err != nil {
		return nil, err
	}
	doc, probs := Validate(pageDoc(k.ID, page), block, nil) // no media: nothing of the owner's images is left
	if len(probs) > 0 {
		return nil, invalid(probs...)
	}
	page = pageOf(doc, k.ID)
	if len(page) > MaxTemplateBytes {
		return nil, apperr.New(apperr.TooLarge, "the page is larger than 65536 bytes")
	}
	now := s.now()
	t := &ownRow{PublicID: ulid.New(now), Name: name, Kind: k.ID, TemplateID: *st.templateID, page: page, ItemCount: itemCount(doc), CreatedAt: now.UnixMilli()}
	if err := s.store.createTemplate(ctx, ownerID, t, MaxOwnTemplates); err != nil {
		return nil, err
	}
	if s.obs.TemplateCreated != nil {
		s.obs.TemplateCreated(stripped)
	}
	return &OwnTemplate{ID: t.PublicID, Name: t.Name, Kind: t.Kind, TemplateID: t.TemplateID, ItemCount: t.ItemCount, Bytes: len(page), CreatedAt: t.CreatedAt, Stripped: stripped}, nil
}

// DeleteTemplate deletes the owner's template; another user's and an unknown id are ERROR_NOT_FOUND. Books that used it do not change.
func (s *Service) DeleteTemplate(ctx context.Context, ownerID uint64, id string) error {
	if err := s.take(ctx, ownerID); err != nil {
		return err
	}
	if err := s.store.deleteTemplate(ctx, ownerID, id); err != nil {
		return err
	}
	if s.obs.TemplateDeleted != nil {
		s.obs.TemplateDeleted()
	}
	return nil
}

// ApplyTemplate replaces pages.<kind> of the book's layout with the page of a shipped or an own template, if baseVersion is the book's
// current version, in one write: the whole document is validated like a restore (an asset or design image that is gone is dropped and
// counted, anything else is ERROR_INVALID_LAYOUT), the replaced document becomes one snapshot of kind apply and the version goes up by one.
// A book without a layout gets one. An own template made on another book template than the book has is ERROR_TEMPLATE_MISMATCH, one of
// another kind ERROR_PARAM, an unknown or another user's ERROR_NOT_FOUND, a stale baseVersion ERROR_VERSION_CONFLICT with the stored document.
// Nothing is written unless everything passes.
func (s *Service) ApplyTemplate(ctx context.Context, ownerID uint64, publicID, kind, source, templateID string, baseVersion int64) (*Restored, error) {
	res, err := s.apply(ctx, ownerID, publicID, kind, source, templateID, baseVersion)
	result := ""
	switch {
	case err == nil:
		result = ApplyOK
	case apperr.Is(err, CodeVersionConflict):
		result = ApplyConflict
	case apperr.Is(err, CodeInvalidLayout):
		result = ApplyInvalid
	case apperr.Is(err, CodeTemplateMismatch), apperr.Is(err, apperr.Conflict): // the second: the book's template changed while we validated
		result = ApplyMismatch
	}
	if result != "" && s.obs.Apply != nil {
		s.obs.Apply(source, result)
	}
	if err == nil && s.obs.Log != nil {
		s.obs.Log.InfoContext(ctx, "page template applied", "yearbook_id", publicID, "kind", kind, "source", source, "from_version", baseVersion, "new_version", res.Version, "dropped", res.Dropped)
	}
	if err != nil {
		return nil, err
	}
	return &res.Restored, nil
}

func (s *Service) apply(ctx context.Context, ownerID uint64, publicID, kind, source, templateID string, baseVersion int64) (*restoreResult, error) {
	switch {
	case baseVersion < 0:
		return nil, apperr.New(apperr.Param, "base_version must be 0 or the version read")
	case source == SourceShipped && !templateIDRe.MatchString(templateID), source == SourceOwn && !ulid.Valid(templateID):
		return nil, apperr.New(apperr.Param, "template_id is not a template id")
	case source != SourceShipped && source != SourceOwn:
		return nil, apperr.New(apperr.Param, `source must be "shipped" or "own"`)
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
	k, ok := block.Kind(kind)
	if !ok {
		return nil, apperr.New(apperr.Param, "kind must be a page kind of the book's template")
	}
	var page json.RawMessage
	switch source {
	case SourceShipped:
		for _, t := range templatesFor(*st.templateID, "") {
			if t.ID == templateID {
				if t.Kind != k.ID {
					return nil, apperr.New(apperr.Param, "the template is for another page kind")
				}
				page = t.Page
			}
		}
		if page == nil {
			return nil, apperr.New(apperr.NotFound, "template not found")
		}
	default:
		t, err := s.store.ownTemplate(ctx, ownerID, templateID)
		if err != nil {
			return nil, err
		}
		if t.TemplateID != *st.templateID {
			return nil, apperr.New(CodeTemplateMismatch, "the template was made on another book template")
		}
		if t.Kind != k.ID {
			return nil, apperr.New(apperr.Param, "the template is for another page kind")
		}
		page = t.page
	}
	if baseVersion != st.version { // a stale editor gets the stored document, not a verdict on a page it has not seen
		return nil, conflictOf(&conflict{row: st.row, version: st.version})
	}
	var cur []byte
	if st.row != nil {
		cur = st.row.layout
	}
	merged, err := withPage(cur, k.ID, page)
	if err != nil {
		return nil, err
	}
	own, err := s.ownImages(ctx, ownerID, publicID, merged)
	if err != nil {
		return nil, err
	}
	doc, dropped, probs := ValidateRestore(merged, block, own)
	if len(probs) > 0 {
		return nil, invalid(probs...)
	}
	// Same rule as restore: the compare with the version, the snapshot and the write happen in one transaction under the yearbook row lock,
	// so a write between our read and the lock is a conflict, never a lost update.
	pol := s.policy()
	w, lost, err := s.store.save(ctx, ownerID, publicID, *st.templateID, baseVersion, doc, KindApply, pol)
	if err != nil {
		return nil, err
	}
	if lost != nil {
		return nil, conflictOf(lost)
	}
	s.written(w)
	return &restoreResult{Restored: Restored{Version: w.version, UpdatedAt: pol.nowMs, Layout: doc, Dropped: dropped}, Replaced: baseVersion}, nil
}

// ownRow is a row of book_template_tab; page is its layout_json.
type ownRow struct {
	PublicID, Name, Kind, TemplateID string
	ItemCount                        int
	CreatedAt                        int64
	page                             []byte
}

// createTemplate inserts t unless the owner already has max templates. The count and the insert run under the user's row lock
// (SELECT ... FOR UPDATE on user_tab, the only lock of this transaction), so two parallel creates cannot both take the last place.
func (s *Store) createTemplate(ctx context.Context, ownerID uint64, t *ownRow, max int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return apperr.Wrap(apperr.Internal, err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	var id uint64
	err = tx.QueryRowContext(ctx, `SELECT id FROM user_tab WHERE id = ? FOR UPDATE`, ownerID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return apperr.Wrap(apperr.Internal, err, "lock user")
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM book_template_tab WHERE owner_id = ?`, ownerID).Scan(&n); err != nil {
		return apperr.Wrap(apperr.Internal, err, "count templates")
	}
	if n >= max {
		return apperr.Wrap(CodeTemplateLimit, &LimitError{Limit: max}, "you already have the most own templates allowed")
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO book_template_tab (public_id, owner_id, name, kind, template_id, layout_json, item_count, bytes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		t.PublicID, ownerID, t.Name, t.Kind, t.TemplateID, t.page, t.ItemCount, len(t.page), t.CreatedAt); err != nil {
		return apperr.Wrap(apperr.Internal, err, "write template")
	}
	return apperr.WithCode(apperr.Internal, tx.Commit())
}

// deleteTemplate deletes the owner's template; no row (unknown id, another user's) is errNotFound-like.
func (s *Store) deleteTemplate(ctx context.Context, ownerID uint64, publicID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM book_template_tab WHERE owner_id = ? AND public_id = ?`, ownerID, publicID)
	if err != nil {
		return apperr.Wrap(apperr.Internal, err, "delete template")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperr.New(apperr.NotFound, "template not found")
	}
	return nil
}

// ownTemplate reads one of the owner's templates with its page.
func (s *Store) ownTemplate(ctx context.Context, ownerID uint64, publicID string) (*ownRow, error) {
	t := &ownRow{PublicID: publicID}
	err := s.db.QueryRowContext(ctx,
		`SELECT name, kind, template_id, item_count, created_at, layout_json FROM book_template_tab WHERE owner_id = ? AND public_id = ?`, ownerID, publicID).
		Scan(&t.Name, &t.Kind, &t.TemplateID, &t.ItemCount, &t.CreatedAt, &t.page)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.New(apperr.NotFound, "template not found")
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "read template")
	}
	return t, nil
}

// ownTemplates lists the owner's templates made on the book template tpl, optionally of one kind, newest first (at most MaxOwnTemplates).
func (s *Store) ownTemplates(ctx context.Context, ownerID uint64, tpl, kind string) ([]PageTemplate, error) {
	q, args := `SELECT public_id, name, kind, item_count, created_at, layout_json FROM book_template_tab WHERE owner_id = ? AND template_id = ?`, []any{ownerID, tpl}
	if kind != "" {
		q, args = q+` AND kind = ?`, append(args, kind)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY created_at DESC, id DESC LIMIT ?`, append(args, MaxOwnTemplates)...)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "list templates")
	}
	defer func() { _ = rows.Close() }()
	var out []PageTemplate
	for rows.Next() {
		t := PageTemplate{Source: SourceOwn}
		var page []byte
		if err := rows.Scan(&t.ID, &t.LabelEN, &t.Kind, &t.ItemCount, &t.CreatedAt, &page); err != nil {
			return nil, apperr.Wrap(apperr.Internal, err, "scan template")
		}
		t.LabelVI, t.Page = t.LabelEN, page
		out = append(out, t)
	}
	return out, apperr.WithCode(apperr.Internal, rows.Err())
}
