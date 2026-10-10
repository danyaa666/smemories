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
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

const (
	// multipartOverhead is room, on top of the file cap, for the multipart framing and any small extra fields.
	multipartOverhead = 64 << 10
	maxParts          = 8
	// The global server timeouts (15 s read, 30 s write) are too short for a 10 MiB upload over a phone
	// connection or a large download, so these routes extend their own deadlines.
	uploadReadTimeout = 2 * time.Minute
	routeWriteTimeout = 2 * time.Minute
)

// Handler serves the media endpoints.
type Handler struct {
	svc            *Service
	maxBytes       int64
	requireUser    func(http.Handler) http.Handler
	allowedOrigins []string
	logger         *slog.Logger
	inflight       chan struct{} // uploads being read or processed; bounds the memory held in request buffers
}

// NewHandler builds the handler. At most 4x the service's processing slots of uploads are in flight at
// once (each buffers up to maxBytes); more get 503 busy.
func NewHandler(svc *Service, maxBytes int64, requireUser func(http.Handler) http.Handler, allowedOrigins []string, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, maxBytes: maxBytes, requireUser: requireUser, allowedOrigins: allowedOrigins, logger: logger,
		inflight: make(chan struct{}, 4*cap(svc.sem))}
}

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle("POST /v1/yearbooks/{id}/media", auth.GuardMultipart(h.allowedOrigins, h.requireUser(
		httpx.WithBodyLimit(h.maxBytes+multipartOverhead, http.HandlerFunc(h.upload)))))
	mux.Handle("GET /v1/yearbooks/{id}/media", h.requireUser(http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/media/{id}/content", h.requireUser(http.HandlerFunc(h.content)))
	mux.Handle("DELETE /v1/media/{id}", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.delete))))
}

func owner(r *http.Request) uint64 {
	u, _ := auth.UserFrom(r.Context())
	return u.InternalID
}

// extend moves the connection deadlines of this request out; servers that do not support it keep the global ones.
func extend(w http.ResponseWriter, read bool) {
	rc := http.NewResponseController(w)
	now := time.Now()
	if read {
		_ = rc.SetReadDeadline(now.Add(uploadReadTimeout))
	}
	_ = rc.SetWriteDeadline(now.Add(routeWriteTimeout))
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	select {
	case h.inflight <- struct{}{}:
		defer func() { <-h.inflight }()
	default:
		w.Header().Set("Retry-After", "2")
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "busy", "too many uploads in progress, retry shortly")
		return
	}
	extend(w, true)
	data, ok := h.readFile(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Upload(r.Context(), owner(r), r.PathValue("id"), data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"media": map[string]any{"id": m.ID, "width": m.Width, "height": m.Height, "bytes": m.Bytes}})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := DefaultListLimit
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > MaxListLimit {
			h.fail(w, r, ValidationError{"invalid_limit"})
			return
		}
		limit = n
	}
	items, next, err := h.svc.List(r.Context(), owner(r), r.PathValue("id"), q.Get("uploader"), limit, q.Get("cursor"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	type item struct {
		ID           string    `json:"id"`
		Width        int       `json:"width"`
		Height       int       `json:"height"`
		Bytes        int       `json:"bytes"`
		UploaderKind string    `json:"uploader_kind"`
		CreatedAt    time.Time `json:"created_at"`
	}
	out := make([]item, len(items))
	for i, m := range items {
		out[i] = item{m.ID, m.Width, m.Height, m.Bytes, m.UploaderKind, m.CreatedAt}
	}
	var nc *string
	if next != "" {
		nc = &next
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Media      []item  `json:"media"`
		NextCursor *string `json:"next_cursor"`
	}{out, nc})
}

// readFile streams the multipart body (nothing spills to disk) and returns the content of the part
// named "file", at most h.maxBytes. The client-supplied file name is never looked at.
func (h *Handler) readFile(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	// The multipart parser does not always wrap a body-cap error, so remember what the body returned.
	body := &errRecorder{r: r.Body}
	r.Body = io.NopCloser(body)
	bodyErr := func(err error) error { return cmp.Or(body.err, err) }
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "expected a multipart/form-data body")
		return nil, false
	}
	for range maxParts {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpx.WriteBodyError(w, r, bodyErr(err))
			return nil, false
		}
		if part.FormName() != "file" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, h.maxBytes+1))
		if err != nil {
			httpx.WriteBodyError(w, r, bodyErr(err))
			return nil, false
		}
		if int64(len(data)) > h.maxBytes {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "file too large")
			return nil, false
		}
		return data, true
	}
	httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "missing form field \"file\"")
	return nil, false
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
	extend(w, false)
	size := r.URL.Query().Get("size")
	switch size {
	case "":
		size = SizeDisplay
	case SizeDisplay, SizeThumb, SizePrint:
	default:
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_size", "size must be thumb, display or print")
		return
	}
	m, data, ct, size, err := h.svc.Content(r.Context(), owner(r), r.PathValue("id"), size)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", ct) // from the database, never from the uploader
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "private, max-age=3600")
	etag := `"` + m.SHA256[:32]
	switch size {
	case SizeThumb:
		etag += "-t"
	case SizePrint:
		etag += "-p"
	}
	hd.Set("ETag", etag+`"`)
	// ServeContent answers Range, If-Range and If-None-Match.
	// ponytail: bytes flow through the API (bandwidth ceiling); M2 serves presigned URLs behind CloudFront.
	http.ServeContent(w, r, "", m.CreatedAt, bytes.NewReader(data))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), owner(r), r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rl RateLimitedError
	var se StorageError
	var ve ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, r, http.StatusBadRequest, ve.Code, "invalid query parameter")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrUnsupported):
		httpx.WriteError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "only JPEG, PNG and WebP images are accepted")
	case errors.Is(err, ErrInvalidImage):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_image", "the image is corrupt or too large")
	case errors.Is(err, ErrQuota):
		httpx.WriteError(w, r, http.StatusConflict, "quota_exceeded", "storage quota reached")
	case errors.As(err, &rl):
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(rl.RetryAfter.Seconds()))))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "too many uploads, try again later")
	case errors.As(err, &se):
		h.logger.Error("media: storage failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		httpx.WriteError(w, r, http.StatusBadGateway, "storage_error", "object storage failed")
	default:
		if !errors.Is(err, context.Canceled) {
			h.logger.Error("media: request failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
