# Findings detail: confirmed medium/high/critical records

Source-only: no target code was executed in this audit. The "observed output" below is the static observation the verifier quoted from source.

## Any registered account can hold every owner-upload slot with slow request bodies

- Fingerprint: `media/create-inflight-gate-before-unpaced-body-read`
- Severity: **medium** (likelihood medium, impact medium), confidence medium

### Description
POST /api/media/create takes one of a small global pool of in-flight upload slots (capacity 4 x SMEM_MEDIA_MAX_CONCURRENT, 8 by default) before it reads the multipart body, and then blocks on the socket while the slot is held. The route has no idle timeout, no minimum body rate and no per-user concurrency limit: the only bound is one absolute 2 minute connection read deadline that slowRoute sets once. A client that sends the headers, the opening multipart boundary and a part header, then stalls or drips one byte every few seconds (a drip also keeps a front-end idle timeout from firing), keeps its slot for the full 2 minutes. Eight such connections from one free account make every other owner's upload fail with 503 ERROR_BUSY for as long as the client reopens them (about every 2 minutes). The per-user upload limiter runs only after the body has been read, so the attacker is never counted. The public submission route avoids this (PaceBody, a per-IP and global connection cap, spooling to disk, no pool slot held while the body arrives); the owner route does not. This record rests on source reading only; no code was run and the hold time was not measured.

### Root cause
The shared in-flight slot is acquired before the body is received, and the body read is neither paced nor counted per user, so the client decides how long a slot is held, up to the 2 minute absolute read deadline, and one account can take every slot.

### Intended behavior
The pool bounds the memory of request buffers for uploads being read or processed (handler.go:49), and docs/media.md allows 2 minutes to receive an owner body, so holding a slot while the body arrives is by design. What is missing is a bound the client cannot stretch by stalling and a per-account share: a stalled or dripping body should be dropped quickly and one account should not be able to occupy the whole shared pool, as the public route does with submitPace and its per-IP and global upload caps (internal/notes/submit.go:32-42, 59, 65).

### Ordered trace
1. `internal/media/handler.go:61` (entrypoint, Handler.Routes): POST /api/media/create is wrapped by auth.GuardMultipart, requireUser and slowRoute(uploadRouteTimeout=150 s, uploadReadTimeout=2 min) around WithBodyLimit(maxBytes+64 KiB, h.create). No pacing or per-user wrapper is present.
2. `internal/auth/csrf.go:39` (propagation, guard): The Origin check runs only when the smem_session cookie is present and compares the client-supplied Origin/Referer with the public allowed-origin list, so a non-browser client with its own session passes. The Content-Type check at line 43 only needs multipart/form-data. Authenticate (internal/auth/service.go:263-299) does not require a verified e-mail, and POST /api/auth/register (auth/handler.go:44, 111-126) sets the session cookie at line 125.
3. `internal/media/handler.go:73` (propagation, slowRoute): slowRoute calls httpx.ExtendDeadlines(w, 2 min, 160 s) once before the handler runs. There is no PaceBody call on this route.
4. `internal/httpx/respond.go:40` (propagation, ExtendDeadlines): rc.SetReadDeadline(now.Add(read)) installs a single absolute read deadline of now+2 min on the connection (the statusWriter wrappers have Unwrap, middleware.go:91, so it reaches the conn). Nothing moves it back toward an idle limit, so a drip never trips it before 2 minutes.
5. `internal/media/handler.go:100` (propagation, Handler.create): A slot of the global h.inflight channel is taken (non-blocking) at lines 100-107 before readForm is called at line 108; a full pool answers 503 ERROR_BUSY with Retry-After to every user. The slot is released only when create returns.
6. `internal/media/handler.go:208` (sink, Handler.readForm): io.ReadAll(io.LimitReader(part, maxBytes+1)) (and the preceding mr.NextPart at line 187) block on the client's socket while the slot is held, until the body completes, the 10 MiB cap is hit, or the 2 minute read deadline fires.

### Evidence
- `internal/media/handler.go:56`: inflight is one channel of capacity uploadQueueFactor*cap(svc.sem) (uploadQueueFactor=4 at line 38), shared by all users; the field comment at line 49 says it covers uploads being read or processed.
- `internal/media/handler.go:100`: The slot is acquired (100-107) before readForm (108) and released by the deferred receive at line 102 only when create returns. No per-user counter exists anywhere in the handler or service.
- `internal/media/handler.go:30`: uploadReadTimeout is 2 minutes; slowRoute (line 73) applies it once as an absolute deadline, not an idle timeout. docs/media.md:15-16 documents '2 min to receive the body'.
- `internal/httpx/respond.go:59`: PaceBody (idle, total and minimum-rate limits) exists but the only non-test caller in the repository is internal/notes/submit.go:65; the media route does not use it.
- `internal/media/service.go:94`: The per-user upload limiter (Take, 60 per 10 min) runs inside Upload after ownedYearbook (line 90), so after readForm has returned the bytes; a stalled upload is never counted. The yearbook_id need not be valid for the attacker to hold a slot.
- `internal/config/config.go:232`: SMEM_MEDIA_MAX_CONCURRENT defaults to 2 (.env.example:92 and docs/production-config.md:106 repeat it), giving an 8-slot pool.
- `internal/media/errors.go:15`: CodeBusy is ERROR_BUSY with HTTP 503; TestCreateBusyWhenTooManyInFlight (internal/media/handler_test.go:164-172) shows a full pool returns 503 with Retry-After.
- `internal/httpx/server.go:16`: The global slow-client timeouts (5 s header, 15 s read) are overridden for this route by the connection read deadline set in ExtendDeadlines; no connection cap, LimitListener or ConnState hook exists in the repository (cmd/smemories-api/main.go:180 uses httpx.NewServer as is).

### Dummy attacker and affected resource
- Attacker (dummy): A free registered user who wants every other owner's photo upload to return 503 ERROR_BUSY.
- Affected resource: the shared in-flight owner-upload slot pool (default 8 slots) and every other owner's `POST /api/media/create`.

### Native input
- 8 concurrent HTTP/1.1 requests: POST /api/media/create with Cookie: smem_session=<the attacker's own session from POST /api/auth/register>, Origin: <an entry of the public allowed-origin list>, Content-Type: multipart/form-data; boundary=B, Content-Length: 10485760; send the opening '--B' line, the part header 'Content-Disposition: form-data; name="file"' and a blank line, then one body byte every ~5 s until the server drops the connection at about 120 s, then reconnect (yearbook_id need not be valid: ownership is checked after the body is read)

### Bounded verification instructions (static)
1. Static check 1: read internal/media/handler.go:95-118 and confirm h.inflight is acquired (100-107) before h.readForm (108), released only on return, and that no per-user counter exists.
2. Static check 2: read handler.go:71-76 and httpx/respond.go:36-45 and confirm slowRoute sets only one SetReadDeadline(now+2 min); grep PaceBody and ExtendDeadlines across the repository and confirm PaceBody is called only from internal/notes/submit.go:65.
3. Static check 3: read internal/media/service.go:89-98 and confirm the per-user limiter runs after readForm returned the bytes.
4. Static check 4: read internal/config/config.go:232 and handler.go:38,56 to get the pool size (4 x 2 = 8); read internal/auth/service.go:263-299 and auth/handler.go:111-126 to confirm a freshly registered, unverified session passes RequireUser.

### Observed output and the invariant it shows
Source-only result: every condition above is a quoted line; no code was run and no hold time was measured. Nothing between the request line and the blocking body read bounds a slot's hold time other than the single 2 minute absolute read deadline, and nothing counts or limits the concurrent bodies of one user, so 8 stalled or dripping bodies from one session occupy the whole default pool.

Invariant: a shared resource's hold time must not be controllable by one unauthenticated-to-the-pool principal, and one account must not occupy the whole pool.

### Conditions and containment
- (authentication_level) Any registered session. Registration is open and e-mail verification is not required for RequireUser; one account is enough because no per-user in-flight cap exists.
- (system_configuration) Default pool of 4 x SMEM_MEDIA_MAX_CONCURRENT = 8 slots; a larger setting scales the number of connections needed linearly and no source-visible per-client connection limit exists.
- (network_routing) No front-end control is visible in the repository (no IaC, ALB or WAF source; the deployment is planned, not built). A request-rate rule does not limit 8 long-lived connections; a front end that closes idle connections would end a fully silent body sooner, which a byte-every-few-seconds drip avoids.
- (timing_dependency) The attacker must reopen the connections about every 2 minutes to keep the pool full; the effect ends within about 2 minutes of the attacker stopping.

### Source-level remediation
Stop letting the client control how long a shared slot is held, and give each account only a share of it. Minimum fix: take a small per-user in-progress cap (for example 2, keyed by u.InternalID) before the global slot, and wrap the body in httpx.PaceBody (idle 10 s, total 2 min, minimum 16 KiB/s after a 15 s grace, as the public route does) so a stalled or dripping body is dropped with 408 within seconds. Do not move the global slot to after the in-memory read: the pool exists to bound request-buffer memory, and acquiring it after io.ReadAll would let many connections each buffer up to 10 MiB without bound. Residual risk of the minimum fix: several accounts could still fill the pool, so the structural fix is to follow internal/notes/submit.go: pace and spool the owner body to a temporary file under its own per-user and global connection caps, and take the memory-bounding slot only when the complete file is loaded for process(), so a slow client costs a socket and some disk, never a shared slot.

Sketch for `internal/media/handler.go` (verifier-proposed, not applied, not compiled):
```go
// userSlots caps the uploads one account may have in progress, so a single session cannot take the shared pool.
type userSlots struct {
	mu  sync.Mutex
	n   map[uint64]int
	max int
}

func (s *userSlots) acquire(id uint64) (release func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.n[id] >= s.max {
		return nil, false
	}
	s.n[id]++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.n[id]--; s.n[id] <= 0 {
			delete(s.n, id)
		}
	}, true
}

// uploadPace: no byte for 10 s, under 16 KiB/s on average after a 15 s grace, or more than uploadReadTimeout in total drops the body (408).
var uploadPace = httpx.BodyPace{Idle: 10 * time.Second, Total: uploadReadTimeout, MinRate: 16 << 10, Grace: 15 * time.Second}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	releaseUser, ok := h.perUser.acquire(u.InternalID) // h.perUser = &userSlots{n: map[uint64]int{}, max: 2}, set in NewHandler
	if !ok {
		h.fail(w, r, tooManyUploads(2*time.Second))
		return
	}
	defer releaseUser()
	select { // the global slot keeps bounding the request buffers in memory, so it stays held while the body is read
	case h.inflight <- struct{}{}:
		defer func() { <-h.inflight }()
	default:
		w.Header().Set("Retry-After", busyRetryAfter)
		h.fail(w, r, ErrBusy)
		return
	}
	httpx.PaceBody(w, r, uploadPace) // a stalled or dripping body is dropped instead of holding the slot until the 2 min deadline
	yearbookID, data, ok := h.readForm(w, r)
	if !ok {
		return
	}
	httpx.ExtendDeadlines(w, uploadProcessing, 0) // PaceBody left the read deadline at the idle limit; move it past the processing phase
	m, err := h.svc.Upload(r.Context(), u.InternalID, yearbookID, data)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, toJSON(m, yearbookID))
}
```

### Regression cases
- Open `max+1` concurrent `POST /api/media/create` bodies from one session that drip bytes; assert the per-user cap refuses the third with 503 and that the pool never reaches capacity from one user.
- Send a body with no bytes for more than the idle limit (10 s); assert the connection is dropped with 408 and the slot is freed.
- Assert an unrelated user's upload still succeeds while the first user's slow bodies are open.

