package media

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

const (
	// multipartOverhead is room, on top of the file cap, for the multipart framing and any small extra fields.
	multipartOverhead = 64 << 10
	maxParts          = 8
	maxIDBytes        = 64 // a yearbook id is 26 characters; anything longer is not one

	// The global server timeouts (15 s read, 30 s write) are too short for a 10 MiB upload over a phone connection or a
	// large download, so these routes carry their own: the request context ends at the route timeout, and the connection's
	// write deadline is moved past it (writeSlack) so the answer to a request that used all its time can still be sent.
	// Without that the server's WriteTimeout would cut the response off first.
	uploadReadTimeout   = 2 * time.Minute
	uploadProcessing    = 30 * time.Second
	uploadRouteTimeout  = uploadReadTimeout + uploadProcessing
	contentRouteTimeout = 2 * time.Minute
	writeSlack          = 10 * time.Second

	busyRetryAfter = "2" // seconds, for a refused upload

	uploadQueueFactor = 4 // uploads in flight per processing slot

	// maxUploadsPerUser is how many uploads one account may have in progress (T-102); the next one is 429 before its body is read.
	maxUploadsPerUser = 2
	userCapRetryAfter = 5 * time.Second
)

// Handler serves the media endpoints: it decodes, validates the shape, calls the Service and writes the answer.
type Handler struct {
	svc            *Service
	maxBytes       int64
	requireUser    func(http.Handler) http.Handler
	allowedOrigins []string
	cursors        *httpx.Cursors
	logger         *slog.Logger
	inflight       chan struct{}  // uploads being read or processed; bounds the memory held in request buffers
	pace           httpx.BodyPace // what the body must keep up with; a field only so that a test can shorten it
	userMu         sync.Mutex
	perUser        map[uint64]int // uploads in progress per internal user id (this process only: with N API tasks the cap is N x 2)
}

// NewHandler builds the handler. At most 4x the service's processing slots of uploads are in flight at
// once (each buffers up to maxBytes); more get 503 ERROR_BUSY.
func NewHandler(svc *Service, maxBytes int64, requireUser func(http.Handler) http.Handler, allowedOrigins []string, cursors *httpx.Cursors, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, maxBytes: maxBytes, requireUser: requireUser, allowedOrigins: allowedOrigins, cursors: cursors, logger: logger,
		inflight: make(chan struct{}, uploadQueueFactor*cap(svc.sem)), pace: httpx.PhoneUploadPace, perUser: map[uint64]int{}}
}

// UploadsInProgress is how many owner uploads hold a shared slot now (for metrics).
func (h *Handler) UploadsInProgress() int { return len(h.inflight) }

// takeUser counts one more upload in progress for user; false when the user is at maxUploadsPerUser. It never waits.
func (h *Handler) takeUser(user uint64) bool {
	h.userMu.Lock()
	defer h.userMu.Unlock()
	if h.perUser == nil {
		h.perUser = map[uint64]int{}
	}
	if h.perUser[user] >= maxUploadsPerUser {
		return false
	}
	h.perUser[user]++
	return true
}

func (h *Handler) releaseUser(user uint64) {
	h.userMu.Lock()
	defer h.userMu.Unlock()
	if h.perUser[user]--; h.perUser[user] <= 0 {
		delete(h.perUser, user)
	}
}

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle("POST /api/media/create", auth.GuardMultipart(h.allowedOrigins, h.requireUser(
		slowRoute(uploadRouteTimeout, uploadReadTimeout, httpx.WithBodyLimit(h.maxBytes+multipartOverhead, http.HandlerFunc(h.create))))))
	mux.Handle("GET /api/media/get-list", h.requireUser(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/media/get-content", h.requireUser(slowRoute(contentRouteTimeout, 0, http.HandlerFunc(h.content))))
	mux.Handle("POST /api/media/delete", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.delete))))
}

// slowRoute gives a route its own request deadline (httpx.WithTimeout) and moves the connection's deadlines out to fit it:
// the write deadline to timeout+writeSlack, the read deadline to read (0 = leave it). A server or writer that does not
// support deadlines keeps the global ones.
func slowRoute(timeout, read time.Duration, next http.Handler) http.Handler {
	return httpx.WithTimeout(timeout, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.ExtendDeadlines(w, read, timeout+writeSlack)
		next.ServeHTTP(w, r)
	}))
}

// mediaJSON is a photo as the API shows it: no storage keys, no URLs (the client builds the content URL from id).
//
// mediaJSON is passed by value on purpose: a response body encoded once.
type mediaJSON struct {
	ID          string `json:"id"`
	YearbookID  string `json:"yearbook_id"`
	ContentType string `json:"content_type"`
	Bytes       int    `json:"bytes"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	CreatedAt   int64  `json:"created_at"` // Unix ms
}

func toJSON(m *Media, yearbookID string) mediaJSON {
	return mediaJSON{m.ID, yearbookID, m.ContentType, m.Bytes, m.Width, m.Height, m.CreatedAt}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	// One account cannot hold the shared pool: its own cap comes first, before any body byte is read (T-102).
	if !h.takeUser(u.InternalID) {
		h.fail(w, r, tooManyUploads(userCapRetryAfter))
		return
	}
	defer h.releaseUser(u.InternalID)
	select {
	case h.inflight <- struct{}{}:
		defer func() { <-h.inflight }()
	default:
		w.Header().Set("Retry-After", busyRetryAfter)
		h.fail(w, r, ErrBusy)
		return
	}
	// The slot stays before the read (it bounds the request buffers); the pace makes a stalled or dripping body end in 408 and free it.
	httpx.PaceBody(w, r, h.pace)
	yearbookID, data, ok := h.readForm(w, r)
	if !ok {
		return
	}
	httpx.ExtendDeadlines(w, uploadProcessing, 0) // the pace left a read deadline 10 s out; processing may take longer and must not look like a gone client
	m, err := h.svc.Upload(r.Context(), u.InternalID, yearbookID, data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, toJSON(m, yearbookID))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	yearbookID := q.Get("yearbook_id")
	if yearbookID == "" {
		h.fail(w, r, apperr.New(apperr.Param, "yearbook_id is required"))
		return
	}
	limit, err := httpx.ParseLimit(r, DefaultListLimit, MaxListLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	scope := "media/get-list:" + yearbookID // a token from another yearbook (or another list) is refused
	var before uint64
	if token := q.Get("next_id"); token != "" {
		pos, err := h.cursors.Open(scope, token)
		if err == nil {
			before, err = parsePosition(pos)
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
	}
	items, next, err := h.svc.List(r.Context(), u.InternalID, yearbookID, q.Get("uploader"), limit, before)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := httpx.Page[mediaJSON]{Items: make([]mediaJSON, len(items))}
	for i := range items {
		page.Items[i] = toJSON(&items[i], yearbookID)
	}
	if next != 0 {
		page.NextID = h.cursors.Seal(scope, strconv.FormatUint(next, 10))
	}
	httpx.OK(w, r, page)
}

// parsePosition reads the position inside a page token. A token we sealed always parses; the check is for a bug or a key
// that signs something else.
func parsePosition(pos string) (uint64, error) {
	n, err := strconv.ParseUint(pos, 10, 63)
	if err != nil || n == 0 {
		return 0, apperr.New(apperr.Param, "next_id is not a valid page token")
	}
	return n, nil
}

// readForm streams the multipart body (nothing spills to disk) and returns the form field "yearbook_id" and the content of
// the part named "file" (at most h.maxBytes), in either order. The client-supplied file name is never looked at.
func (h *Handler) readForm(w http.ResponseWriter, r *http.Request) (yearbookID string, data []byte, ok bool) {
	// The multipart parser does not always wrap a body-cap error, so remember what the body returned.
	body := &errRecorder{r: r.Body}
	r.Body = io.NopCloser(body)
	bodyErr := func(err error) error { return cmp.Or(body.err, err) }
	mr, err := r.MultipartReader()
	if err != nil {
		h.fail(w, r, apperr.New(apperr.Param, "expected a multipart/form-data body"))
		return "", nil, false
	}
	haveFile := false
	for range maxParts {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpx.WriteBodyError(w, r, bodyErr(err))
			return "", nil, false
		}
		switch name := part.FormName(); {
		case name == "yearbook_id" && yearbookID == "":
			b, err := io.ReadAll(io.LimitReader(part, maxIDBytes+1))
			if err != nil {
				httpx.WriteBodyError(w, r, bodyErr(err))
				return "", nil, false
			}
			if len(b) > maxIDBytes {
				h.fail(w, r, apperr.New(apperr.Param, "yearbook_id is not a yearbook id"))
				return "", nil, false
			}
			yearbookID = string(b)
		case name == "file" && !haveFile:
			data, err = io.ReadAll(io.LimitReader(part, h.maxBytes+1))
			if err != nil {
				httpx.WriteBodyError(w, r, bodyErr(err))
				return "", nil, false
			}
			if int64(len(data)) > h.maxBytes {
				h.fail(w, r, apperr.New(apperr.TooLarge, "file too large"))
				return "", nil, false
			}
			haveFile = true
		}
		if haveFile && yearbookID != "" {
			break
		}
	}
	switch {
	case yearbookID == "":
		h.fail(w, r, apperr.New(apperr.Param, "missing form field \"yearbook_id\""))
	case !haveFile:
		h.fail(w, r, apperr.New(apperr.Param, "missing form field \"file\""))
	default:
		return yearbookID, data, true
	}
	return "", nil, false
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

func (h *Handler) content(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		h.fail(w, r, apperr.New(apperr.Param, "id is required"))
		return
	}
	variant := q.Get("variant")
	switch variant {
	case "":
		variant = SizeDisplay
	case SizeDisplay, SizeThumb, SizePrint:
	default:
		h.fail(w, r, apperr.New(apperr.Param, "variant must be thumb, display or print"))
		return
	}
	m, data, ct, variant, err := h.svc.Content(r.Context(), u.InternalID, id, variant)
	if err != nil {
		h.fail(w, r, err) // an error is the JSON envelope with no-store (SecurityHeaders): an <img> only needs the status, and nothing is cached
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", ct) // from the database, never from the uploader
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "private, max-age=3600")
	etag := `"` + m.SHA256[:32]
	switch variant {
	case SizeThumb:
		etag += "-t"
	case SizePrint:
		etag += "-p"
	}
	hd.Set("ETag", etag+`"`)
	// ServeContent answers Range, If-Range and If-None-Match.
	// ponytail: bytes flow through the API (bandwidth ceiling); M2 serves presigned URLs behind CloudFront.
	http.ServeContent(w, r, "", time.UnixMilli(m.CreatedAt).UTC(), bytes.NewReader(data))
}

type deleteInput struct {
	ID string `json:"id"`
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in deleteInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if in.ID == "" {
		h.fail(w, r, apperr.New(apperr.Param, "id is required"))
		return
	}
	if err := h.svc.Delete(r.Context(), u.InternalID, in.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, nil)
}

// fail answers err. A *RateLimitedError sets Retry-After; a storage failure is logged (Fail logs only the codes that are
// our fault by default); the rest, a gone client included, is httpx.Fail.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rl *RateLimitedError
	if errors.As(err, &rl) {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(rl.RetryAfter.Seconds()))))
	}
	if apperr.Is(err, CodeStorage) && !errors.Is(err, context.Canceled) {
		h.logger.Error("media: storage failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
	}
	httpx.Fail(w, r, err)
}
