package notes

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
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

	// Slow phone uploads: the body is spooled to disk while it arrives, so a slow client costs a socket and some disk,
	// never memory or a processing slot. It must keep moving (submitPace), then get at most submitProcessing for the
	// photos. This route extends its own deadlines instead of raising the global ones.
	submitProcessing   = 60 * time.Second
	submitRouteTimeout = 120*time.Second + submitProcessing
	submitWriteTimeout = submitRouteTimeout + 10*time.Second

	// Submission limits count requests that passed text validation. They are sized for a class behind one
	// campus address (40 students each submit once); the per-collection limit and the 300-note cap are the real brakes.
	submitPerIPHour         = 100
	submitPerIPDay          = 300
	submitPerCollectionHour = 60
)

// submitPace is a variable only so that a test can shorten it: 10 s without a byte, 120 s in total, and after a
// 15 s grace period at least 16 KiB/s on average.
var submitPace = httpx.BodyPace{Idle: 10 * time.Second, Total: 120 * time.Second, MinRate: 16 << 10, Grace: 15 * time.Second}

// plainID matches the ids worth echoing back in an error; an unknown id chosen by the client is not echoed otherwise.
var plainID = regexp.MustCompile(`^[a-z_]{1,40}$`)

// retryJitter is the Retry-After of a 503 busy: 2 to 6 seconds at random, so a class that was turned away does not
// come back in one wave.
func retryJitter() string { return strconv.Itoa(2 + rand.IntN(5)) } //nolint:gosec // spreading retries, not a secret

// scan is what the multipart body held, without the photo bytes.
type scan struct {
	answers    []byte
	hasAnswers bool
	honeypot   bool
	photos     []int // sequence numbers of the parts that hold a photo
	photoFirst bool  // a photo came before the answers part
}

// submit stores a friend's note as pending. Order matters: a slot for the network phase is taken before anything
// is read; the body is spooled to disk; the link is checked and the text validated before any photo is decoded;
// every failure after a photo was stored deletes what was stored.
func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	ip := h.clientIP(r)
	if !h.acquire(ip) { // nothing was read or counted, so a busy answer costs the client nothing
		busy(w, r)
		return
	}
	defer h.release(ip)
	httpx.ExtendDeadlines(w, 0, submitWriteTimeout)
	httpx.PaceBody(w, r, submitPace)
	rec := &errRecorder{r: r.Body}
	r.Body = io.NopCloser(rec)
	w = &drainWriter{ResponseWriter: w, rec: rec} // an early answer first swallows the rest of the body, so a browser can read it

	p, ok := h.resolve(w, r)
	if !ok {
		return
	}
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		httpx.WriteError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "send multipart/form-data")
		return
	}
	if params["boundary"] == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "expected a multipart/form-data body")
		return
	}
	if n, err := h.store.noteCount(r.Context(), p.CollectionID); err != nil || n >= maxNotes {
		h.fail(w, r, cmp.Or(err, ErrFull))
		return
	}

	f, err := os.CreateTemp(h.tmpDir, "smem-upload-*") // 0600
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { // also on error, panic and client disconnect
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	if _, err := io.Copy(f, r.Body); err != nil {
		if rec.err != nil {
			httpx.WriteBodyError(w, r, rec.err)
		} else {
			h.fail(w, r, err) // the disk
		}
		return
	}
	httpx.ExtendDeadlines(w, 0, submitProcessing)

	sc, ok := h.scan(w, r, f, params["boundary"])
	if !ok {
		return
	}
	now := h.now().UTC()
	id := ulid.New(now)
	if sc.honeypot { // a bot filled the hidden field: look successful, keep nothing
		writeCreated(w, id)
		return
	}
	if sc.photoFirst || (len(sc.photos) > 0 && !sc.hasAnswers) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "send the answers part before the photos")
		return
	}
	if !sc.hasAnswers {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "missing form field \"answers\"")
		return
	}
	clean, ok := h.validate(w, r, p, sc.answers)
	if !ok {
		return
	}
	if err := h.verifier.Verify(r.Context(), r); err != nil {
		httpx.WriteError(w, r, http.StatusForbidden, "verification_failed", "verification failed")
		return
	}
	coll := strconv.FormatUint(p.CollectionID, 10)
	if ok, wait := h.takeSubmit(ip, coll); !ok {
		tooMany(w, r, wait)
		return
	}

	var stored []media.Media
	fail := func(err error) {
		if errors.Is(err, media.ErrBusy) { // turned away, not misused: give back what the request cost
			h.all.Refund(ip)
			h.subIPHr.Refund(ip)
			h.subIPDay.Refund(ip)
			h.subColl.Refund(coll)
		}
		if derr := h.media.Discard(stored...); derr != nil {
			h.logger.Error("notes: could not remove the photos of a failed submission", "request_id", httpx.RequestIDFrom(r.Context()), "error", derr)
		}
		h.fail(w, r, err)
	}
	rows := make([]int64, 0, len(sc.photos))
	if len(sc.photos) > 0 {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			fail(err)
			return
		}
		mr := multipart.NewReader(f, params["boundary"])
		for n, i := 0, 0; i < len(sc.photos); n++ { // one photo at a time; the media service bounds decoding and memory
			part, err := mr.NextPart()
			if err != nil {
				fail(err) // cannot happen: the scan read the same bytes
				return
			}
			if n != sc.photos[i] {
				continue
			}
			i++
			m, err := h.media.UploadContributorFrom(r.Context(), p.YearbookID, func() ([]byte, error) {
				return io.ReadAll(io.LimitReader(part, h.maxPhoto+1))
			})
			if err != nil {
				fail(err)
				return
			}
			stored = append(stored, m)
			rows = append(rows, m.RowID)
		}
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

func busy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", retryJitter())
	httpx.WriteError(w, r, http.StatusServiceUnavailable, "busy", "too many uploads in progress, retry shortly")
}

func writeCreated(w http.ResponseWriter, id string) {
	type note struct {
		ID string `json:"id"`
	}
	httpx.WriteJSON(w, http.StatusCreated, struct {
		Note note `json:"note"`
	}{note{id}})
}

// scan walks the spooled body once and answers the error itself (false) when the structure is wrong: a part too
// large, too many photos, a body that is not multipart. Photos are only measured here, never decoded.
// A filled honeypot field ends the scan early with sc.honeypot set.
func (h *Handler) scan(w http.ResponseWriter, r *http.Request, f *os.File, boundary string) (*scan, bool) {
	sc := &scan{}
	invalid := func(msg string) (*scan, bool) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", msg)
		return nil, false
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		h.fail(w, r, err)
		return nil, false
	}
	mr := multipart.NewReader(f, boundary)
	for n := 0; ; n++ {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("the multipart body is malformed or incomplete")
		}
		if n >= maxParts {
			return invalid("too many parts")
		}
		switch part.FormName() { // the file name and the part's Content-Type are never looked at
		case "answers":
			b, err := io.ReadAll(io.LimitReader(part, maxAnswersBytes+1))
			if err != nil {
				return invalid("the multipart body is malformed or incomplete")
			}
			if sc.hasAnswers || len(b) > maxAnswersBytes {
				return invalid("answers must be sent once and be at most 16 KiB")
			}
			sc.answers, sc.hasAnswers = b, true
			sc.photoFirst = len(sc.photos) > 0
		case "website":
			b, err := io.ReadAll(io.LimitReader(part, maxHoneypot))
			if err != nil {
				return invalid("the multipart body is malformed or incomplete")
			}
			if len(b) > 0 {
				sc.honeypot = true
				return sc, true
			}
		case "photos":
			size, err := io.Copy(io.Discard, io.LimitReader(part, h.maxPhoto+1))
			if err != nil {
				return invalid("the multipart body is malformed or incomplete")
			}
			if size > h.maxPhoto {
				httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "photo too large")
				return nil, false
			}
			if size == 0 && part.FileName() == "" { // a browser's empty file input
				continue
			}
			if len(sc.photos) == maxPhotos {
				httpx.WriteError(w, r, http.StatusBadRequest, "too_many_photos", "at most 3 photos")
				return nil, false
			}
			sc.photos = append(sc.photos, n)
		}
	}
	return sc, true
}

// acquire takes a submission slot for ip: one of upMax in total (each holds a socket and up to 32 MiB of disk) and
// one of the ip's own upPerIP, so one address cannot use them all. It never waits.
func (h *Handler) acquire(ip string) bool {
	h.upMu.Lock()
	defer h.upMu.Unlock()
	if h.upTotal >= h.upMax || h.upIP[ip] >= h.upPerIP {
		return false
	}
	h.upTotal++
	h.upIP[ip]++
	return true
}

// release gives back what acquire took.
func (h *Handler) release(ip string) {
	h.upMu.Lock()
	defer h.upMu.Unlock()
	h.upTotal--
	if h.upIP[ip]--; h.upIP[ip] <= 0 {
		delete(h.upIP, ip)
	}
}

// validate parses the answers part and checks it against the form's fields. It returns the cleaned answers.
func (h *Handler) validate(w http.ResponseWriter, r *http.Request, p Public, raw []byte) (map[string]string, bool) {
	var answers map[string]string
	if b := bytes.TrimSpace(raw); !utf8.Valid(b) || len(b) == 0 || b[0] != '{' || json.Unmarshal(b, &answers) != nil {
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

// drainWriter reads the rest of the request body, once, before the first byte of the answer is written. An answer
// sent while the client is still uploading is often not shown by a browser (it sees the connection reset instead),
// so the early rejections (closed link, bad text, rate limit) wait for the upload to end; the pace and size limits
// of the body still apply. A body that already failed is not read again.
type drainWriter struct {
	http.ResponseWriter
	rec     *errRecorder
	drained bool
}

func (d *drainWriter) drain() {
	if !d.drained {
		d.drained = true
		if d.rec.err == nil {
			_, _ = io.Copy(io.Discard, d.rec)
		}
	}
}

func (d *drainWriter) WriteHeader(code int) {
	d.drain()
	d.ResponseWriter.WriteHeader(code)
}

func (d *drainWriter) Write(b []byte) (int, error) {
	d.drain()
	return d.ResponseWriter.Write(b)
}

func (d *drainWriter) Unwrap() http.ResponseWriter { return d.ResponseWriter }

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
