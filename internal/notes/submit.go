package notes

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notefields"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	maxBody         = 32 << 20 // whole request
	maxAnswersBytes = 16 << 10
	maxPhotos       = 3
	maxParts        = 12 // answers, website, photos and a few strays; more is hostile
	maxHoneypot     = 1 << 10

	// Slow phone uploads: this route extends its own deadlines instead of raising the global ones.
	submitReadTimeout  = 120 * time.Second
	submitWriteTimeout = 120 * time.Second

	// Submission limits count requests that passed text validation. They are sized for a class behind one
	// campus address (40 students each submit once); the per-collection limit and the 300-note cap are the real brakes.
	submitPerIPHour         = 100
	submitPerIPDay          = 300
	submitPerCollectionHour = 60
)

// plainID matches the ids worth echoing back in an error; an unknown id chosen by the client is not echoed otherwise.
var plainID = regexp.MustCompile(`^[a-z_]{1,40}$`)

// submission is what the multipart body held. Photo bytes stay in memory only until they are processed.
type submission struct {
	answers    []byte
	hasAnswers bool
	honeypot   bool
	photos     [][]byte
	held       bool // an inflight slot is taken
}

// submit stores a friend's note as pending. Order matters: the link is checked before the body is read, text is
// validated before any photo is decoded, and every failure after a photo was stored deletes what was stored.
func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	p, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "multipart/form-data" {
		httpx.WriteError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "send multipart/form-data")
		return
	}
	if n, err := h.store.noteCount(r.Context(), p.CollectionID); err != nil || n >= maxNotes {
		h.fail(w, r, cmp.Or(err, ErrFull))
		return
	}
	httpx.ExtendDeadlines(w, submitReadTimeout, submitWriteTimeout)

	sub := &submission{}
	defer func() {
		if sub.held {
			<-h.inflight
		}
	}()
	if !h.readParts(w, r, sub) {
		return
	}
	now := h.now().UTC()
	id := ulid.New(now)
	if sub.honeypot { // a bot filled the hidden field: look successful, keep nothing
		writeCreated(w, id)
		return
	}
	if err := h.verifier.Verify(r.Context(), r); err != nil {
		httpx.WriteError(w, r, http.StatusForbidden, "verification_failed", "verification failed")
		return
	}
	clean, ok := h.validate(w, r, p, sub)
	if !ok {
		return
	}
	if ok, wait := h.takeSubmit(h.clientIP(r), strconv.FormatUint(p.CollectionID, 10)); !ok {
		tooMany(w, r, wait)
		return
	}

	var stored []media.Media
	fail := func(err error) {
		if derr := h.media.Discard(stored...); derr != nil {
			h.logger.Error("notes: could not remove the photos of a failed submission", "request_id", httpx.RequestIDFrom(r.Context()), "error", derr)
		}
		h.fail(w, r, err)
	}
	rows := make([]int64, 0, len(sub.photos))
	for i, data := range sub.photos { // one at a time: the media service bounds decoding and memory
		m, err := h.media.UploadContributor(r.Context(), p.YearbookID, data)
		sub.photos[i] = nil
		if err != nil {
			fail(err)
			return
		}
		stored = append(stored, m)
		rows = append(rows, m.RowID)
	}
	answers, err := json.Marshal(clean)
	if err == nil {
		err = h.store.insertNote(r.Context(), newNote{ID: id, Answers: answers, MediaRows: rows, CreatedAt: now, Collection: p.CollectionID, Yearbook: p.YearbookID})
	}
	if err != nil {
		fail(err)
		return
	}
	writeCreated(w, id)
}

func writeCreated(w http.ResponseWriter, id string) {
	type note struct {
		ID string `json:"id"`
	}
	httpx.WriteJSON(w, http.StatusCreated, struct {
		Note note `json:"note"`
	}{note{id}})
}

// readParts reads the multipart body into sub. It answers the error itself and returns false on failure.
// A filled honeypot field stops the read early with sub.honeypot set.
func (h *Handler) readParts(w http.ResponseWriter, r *http.Request, sub *submission) bool {
	// The multipart parser does not always wrap a body-cap error, so remember what the body returned.
	body := &errRecorder{r: r.Body}
	r.Body = io.NopCloser(body)
	bodyErr := func(err error) error { return cmp.Or(body.err, err) }
	invalid := func(msg string) bool {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", msg)
		return false
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return invalid("expected a multipart/form-data body")
	}
	for n := 0; ; n++ {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpx.WriteBodyError(w, r, bodyErr(err))
			return false
		}
		if n >= maxParts {
			return invalid("too many parts")
		}
		switch part.FormName() { // the file name and the part's Content-Type are never looked at
		case "answers":
			b, err := io.ReadAll(io.LimitReader(part, maxAnswersBytes+1))
			if err != nil {
				httpx.WriteBodyError(w, r, bodyErr(err))
				return false
			}
			if sub.hasAnswers || len(b) > maxAnswersBytes {
				return invalid("answers must be sent once and be at most 16 KiB")
			}
			sub.answers, sub.hasAnswers = b, true
		case "website":
			b, err := io.ReadAll(io.LimitReader(part, maxHoneypot))
			if err != nil {
				httpx.WriteBodyError(w, r, bodyErr(err))
				return false
			}
			if len(b) > 0 {
				sub.honeypot = true
				return true
			}
		case "photos":
			if len(sub.photos) == maxPhotos {
				var probe [1]byte
				if k, _ := io.ReadFull(part, probe[:]); k > 0 {
					httpx.WriteError(w, r, http.StatusBadRequest, "too_many_photos", "at most 3 photos")
					return false
				}
				continue
			}
			if !sub.held {
				select {
				case h.inflight <- struct{}{}:
					sub.held = true
				default:
					w.Header().Set("Retry-After", "2")
					httpx.WriteError(w, r, http.StatusServiceUnavailable, "busy", "too many uploads in progress, retry shortly")
					return false
				}
			}
			data, err := io.ReadAll(io.LimitReader(part, h.maxPhoto+1))
			if err != nil {
				httpx.WriteBodyError(w, r, bodyErr(err))
				return false
			}
			if int64(len(data)) > h.maxPhoto {
				httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "photo too large")
				return false
			}
			if len(data) == 0 && part.FileName() == "" { // a browser's empty file input
				continue
			}
			sub.photos = append(sub.photos, data)
		}
	}
	if !sub.hasAnswers {
		return invalid("missing form field \"answers\"")
	}
	return true
}

// validate parses the answers part and checks it against the form's fields. It returns the cleaned answers.
func (h *Handler) validate(w http.ResponseWriter, r *http.Request, p Public, sub *submission) (map[string]string, bool) {
	var answers map[string]string
	if b := bytes.TrimSpace(sub.answers); !utf8.Valid(b) || len(b) == 0 || b[0] != '{' || json.Unmarshal(b, &answers) != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "answers must be a JSON object of strings")
		return nil, false
	}
	refs, err := FieldsFor(r.Context(), p.YearbookID)
	var clean map[string]string
	if err == nil {
		clean, err = notefields.Validate(refs, answers)
	}
	var fe *notefields.FieldError
	if errors.As(err, &fe) {
		msg := fe.Code // never the value; the id only when it is one of ours or plainly harmless
		if fe.Code != notefields.CodeUnknownField || plainID.MatchString(fe.ID) {
			msg += ": " + fe.ID
		}
		httpx.WriteError(w, r, http.StatusBadRequest, fe.Code, msg)
		return nil, false
	}
	if err != nil {
		h.fail(w, r, err)
		return nil, false
	}
	return clean, true
}

// takeSubmit counts a submission against the per-IP and per-collection limits; none is counted unless all allow it.
func (h *Handler) takeSubmit(ip, collection string) (bool, time.Duration) {
	if ok, wait := h.subIPHr.Take(ip); !ok {
		return false, wait
	}
	if ok, wait := h.subIPDay.Take(ip); !ok {
		h.subIPHr.Refund(ip)
		return false, wait
	}
	if ok, wait := h.subColl.Take(collection); !ok {
		h.subIPHr.Refund(ip)
		h.subIPDay.Refund(ip)
		return false, wait
	}
	return true, 0
}

// errRecorder remembers the first non-EOF error of the reader it wraps.
type errRecorder struct {
	r   io.Reader
	err error
}

func (e *errRecorder) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err != nil && err != io.EOF && e.err == nil {
		e.err = err
	}
	return n, err
}

// mediaStatus maps the errors of the photo pipeline to the shared envelope; ok is false for any other error.
func mediaStatus(err error) (status int, code, msg string, ok bool) {
	var se media.StorageError
	switch {
	case errors.Is(err, media.ErrUnsupported):
		return http.StatusUnsupportedMediaType, "unsupported_media_type", "only JPEG, PNG and WebP images are accepted", true
	case errors.Is(err, media.ErrInvalidImage):
		return http.StatusBadRequest, "invalid_image", "the image is corrupt or too large", true
	case errors.Is(err, media.ErrQuota):
		return http.StatusConflict, "quota_exceeded", "storage quota reached", true
	case errors.Is(err, media.ErrBusy):
		return http.StatusServiceUnavailable, "busy", "too many uploads in progress, retry shortly", true
	case errors.As(err, &se):
		return http.StatusBadGateway, "storage_error", "object storage failed", true
	}
	return 0, "", "", false
}
