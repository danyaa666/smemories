package notes

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	maxBody         = 32 << 20 // whole request
	maxAnswersBytes = 16 << 10
	maxPhotos       = 3
	maxParts        = 12 // token, answers, website, photos and a few strays; more is hostile
	maxHoneypot     = 1 << 10

	// Slow phone uploads: the body is spooled to disk while it arrives, so a slow client costs a socket and some disk,
	// never memory or a processing slot. It must keep moving (submitPace), then get at most submitProcessing for the
	// photos. This route extends its own deadlines instead of raising the global ones.
	submitProcessing   = 60 * time.Second
	submitRouteTimeout = 120*time.Second + submitProcessing
	submitWriteTimeout = submitRouteTimeout + 10*time.Second
)

// submitPace is a variable only so that a test can shorten it: 10 s without a byte, 120 s in total, and after a
// 15 s grace period at least 16 KiB/s on average.
var submitPace = httpx.BodyPace{Idle: 10 * time.Second, Total: 120 * time.Second, MinRate: 16 << 10, Grace: 15 * time.Second}

// scan is what the multipart body held, without the photo bytes.
type scan struct {
	token      string
	hasToken   bool
	answers    []byte
	hasAnswers bool
	honeypot   bool
	photos     []int // sequence numbers of the parts that hold a photo
	photoFirst bool  // a photo came before the answers part
}

// submit stores a friend's note as pending. Order matters: a slot for the network phase is taken before anything
// is read; the body is spooled to disk (the token is one of its fields); the link is checked and the text validated
// before any photo is decoded; every failure after a photo was stored deletes what was stored.
func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	ip := h.clientIP(r)
	if !h.acquire(ip) { // nothing was read or counted, so a busy answer costs the client nothing
		h.fail(w, r, &RetryAfterError{Err: apperr.New(CodeBusy, "too many uploads in progress, retry shortly"), After: retryJitter()})
		return
	}
	defer h.release(ip)
	httpx.ExtendDeadlines(w, 0, submitWriteTimeout)
	httpx.PaceBody(w, r, submitPace)
	rec := &errRecorder{r: r.Body}
	r.Body = io.NopCloser(rec)
	w = &drainWriter{ResponseWriter: w, rec: rec} // an early answer first swallows the rest of the body, so a browser can read it

	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		h.fail(w, r, apperr.New(CodeUnsupportedMediaType, "send multipart/form-data"))
		return
	}
	if params["boundary"] == "" {
		h.fail(w, r, apperr.New(apperr.Param, "expected a multipart/form-data body"))
		return
	}

	f, err := os.CreateTemp(h.tmpDir, "smem-upload-*") // 0600
	if err != nil {
		h.fail(w, r, apperr.Wrap(apperr.Internal, err, "create upload spool file"))
		return
	}
	defer func() { // also on error, panic and client disconnect
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	size, err := io.Copy(f, r.Body)
	if err != nil {
		if rec.err != nil {
			httpx.WriteBodyError(w, r, rec.err)
		} else {
			h.fail(w, r, apperr.Wrap(apperr.Internal, err, "spool upload")) // the disk
		}
		return
	}
	httpx.ExtendDeadlines(w, 0, submitProcessing)

	sc, err := h.scan(f, params["boundary"])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if sc.honeypot { // a bot filled the hidden field: look successful, keep nothing
		httpx.OK(w, r, noteCreated{ulid.New(time.Now())})
		return
	}
	p, err := h.svc.Resolve(r.Context(), ip, sc.token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if sc.photoFirst || (len(sc.photos) > 0 && !sc.hasAnswers) {
		h.fail(w, r, apperr.New(apperr.Param, "send the answers part before the photos"))
		return
	}
	if !sc.hasAnswers {
		h.fail(w, r, apperr.New(apperr.Param, "missing form field \"answers\""))
		return
	}
	in := &Submission{IP: ip, Answers: sc.answers, Photos: make([]func() ([]byte, error), len(sc.photos)),
		Verify: func(ctx context.Context) error { return h.verifier.Verify(ctx, r) }}
	for i, n := range sc.photos {
		in.Photos[i] = h.loadPhoto(f, size, params["boundary"], n)
	}
	id, err := h.svc.SubmitNote(r.Context(), p, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, noteCreated{id})
}

type noteCreated struct {
	ID string `json:"id"`
}

// loadPhoto returns the loader of the photo that is part number n of the spooled body; it reads the part when called.
func (h *Handler) loadPhoto(f *os.File, size int64, boundary string, n int) func() ([]byte, error) {
	return func() ([]byte, error) {
		mr := multipart.NewReader(io.NewSectionReader(f, 0, size), boundary)
		for i := 0; ; i++ {
			part, err := mr.NextPart()
			if err != nil {
				return nil, apperr.Wrap(apperr.Internal, err, "read spooled photo") // cannot happen: the scan read the same bytes
			}
			if i == n {
				return io.ReadAll(io.LimitReader(part, h.maxPhoto+1))
			}
		}
	}
}

// retryJitter is the Retry-After of a 503 busy: 2 to 6 seconds at random, so a class that was turned away does not
// come back in one wave.
func retryJitter() time.Duration {
	return time.Duration(2+rand.IntN(5)) * time.Second //nolint:gosec // spreading retries, not a secret
}

// scan walks the spooled body once and returns the error to answer (false) when the structure is wrong: a part too
// large, too many photos, a body that is not multipart. Photos are only measured here, never decoded.
// A filled honeypot field ends the scan early with sc.honeypot set.
func (h *Handler) scan(f *os.File, boundary string) (*scan, error) {
	sc := &scan{}
	malformed := apperr.New(apperr.Param, "the multipart body is malformed or incomplete")
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "rewind spooled upload")
	}
	mr := multipart.NewReader(f, boundary)
	for n := 0; ; n++ {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, malformed
		}
		if n >= maxParts {
			return nil, apperr.New(apperr.Param, "too many parts")
		}
		switch part.FormName() { // the file name and the part's Content-Type are never looked at
		case "token":
			b, err := io.ReadAll(io.LimitReader(part, tokenLen+1)) // one byte over: it can only be a miss
			if err != nil {
				return nil, malformed
			}
			if !sc.hasToken {
				sc.token, sc.hasToken = string(b), true
			}
		case "answers":
			b, err := io.ReadAll(io.LimitReader(part, maxAnswersBytes+1))
			if err != nil {
				return nil, malformed
			}
			if sc.hasAnswers || len(b) > maxAnswersBytes {
				return nil, apperr.New(apperr.Param, "answers must be sent once and be at most 16 KiB")
			}
			sc.answers, sc.hasAnswers = b, true
			sc.photoFirst = len(sc.photos) > 0
		case "website":
			b, err := io.ReadAll(io.LimitReader(part, maxHoneypot))
			if err != nil {
				return nil, malformed
			}
			if len(b) > 0 {
				sc.honeypot = true
				return sc, nil
			}
		case "photos":
			size, err := io.Copy(io.Discard, io.LimitReader(part, h.maxPhoto+1))
			if err != nil {
				return nil, malformed
			}
			if size > h.maxPhoto {
				return nil, apperr.New(apperr.TooLarge, "photo too large")
			}
			if size == 0 && part.FileName() == "" { // a browser's empty file input
				continue
			}
			if len(sc.photos) == maxPhotos {
				return nil, apperr.New(CodeTooManyPhotos, "at most 3 photos")
			}
			sc.photos = append(sc.photos, n)
		}
	}
	return sc, nil
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
