package class

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ulid"
)

// The invite link (T-125, DESIGN.md): a bearer secret that lets whoever holds it join the class as a student. The token is 32
// random bytes (256 bits, base64url, 43 characters), shown once in the create answer and never stored: class_invite_tab keeps
// its SHA-256. It is never logged, never part of an error, and never a metric label; the handlers read it from a JSON body, so it
// is not in a URL the access log or a proxy could record (the page link /join/<token> is the web's, see docs/class-permissions.md).
const (
	// MaxActiveInvites is the most unexpired, unrevoked invites a class can have at once.
	MaxActiveInvites = 5
	// DefaultInviteLifetime and MaxInviteLifetime bound expires_at.
	DefaultInviteLifetime = 14 * 24 * time.Hour
	MaxInviteLifetime     = 30 * 24 * time.Hour

	DefaultInviteListLimit = 20
	MaxInviteListLimit     = 50

	tokenBytes = 32
	tokenLen   = 43 // base64url of tokenBytes, no padding

	// Public lookups, per client IP: every request counts against lookupAllPerWindow, a miss also against lookupMissesPerWindow
	// (a hit is refunded), so a visitor with a good link is never throttled by the guessing limit. Accepting is a write: per user
	// (the shared class write budget) and per IP. The per-IP budgets are generous on purpose: a whole class of 60 students joins
	// from one school network (one NAT address) and must have room for refreshes and retries; a 256-bit token cannot be guessed,
	// so these only bound load. The guessing limit is the miss limiter, which stays at 20.
	lookupAllPerWindow    = 600
	lookupMissesPerWindow = 20
	lookupWindow          = 10 * time.Minute
	acceptPerIPWindow     = 300
	acceptWindow          = time.Hour

	joinPath = "/join/"
)

// Invite is a link as the admin sees it: never the token.
type Invite struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"` // Unix ms
	ExpiresAt int64  `json:"expires_at"` // Unix ms
	RevokedAt *int64 `json:"revoked_at"` // Unix ms, null while not revoked
	Active    bool   `json:"active"`     // not revoked and not expired now
	row       uint64
}

// CreatedInvite is the one answer that carries the token and the ready-made link.
type CreatedInvite struct {
	Invite
	Token string `json:"token"`
	Link  string `json:"link"`
}

// InvitePreview is all a visitor with a valid token learns about the class.
type InvitePreview struct {
	Name       string `json:"name"`
	SchoolName string `json:"school_name"`
}

// SetPublicBaseURL sets the origin invite links are built on (SMEM_PUBLIC_BASE_URL). Unset, the link is the relative /join/<token>.
func (s *Service) SetPublicBaseURL(base string) { s.baseURL = strings.TrimRight(base, "/") }

// newInviteToken returns a fresh token and the SHA-256 that is stored in its place.
func newInviteToken() (token string, hash [sha256.Size]byte, err error) {
	var b [tokenBytes]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", hash, err
	}
	token = base64.RawURLEncoding.EncodeToString(b[:])
	return token, sha256.Sum256([]byte(token)), nil
}

// inviteHash is the stored hash for a token from the client; ok is false when s cannot be one of ours (wrong length or
// alphabet), so garbage never reaches the database. The caller answers that exactly like an unknown token.
func inviteHash(s string) (h [sha256.Size]byte, ok bool) {
	if len(s) != tokenLen {
		return h, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return h, false
		}
	}
	return sha256.Sum256([]byte(s)), true
}

func tooMany(wait time.Duration) error {
	return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many requests, try again later"), After: wait}
}

// CreateInvite makes a link for the class (admin): expiresAt (Unix ms, nil = 14 days) must be in the future and at most 30 days
// away. At most MaxActiveInvites are active at once; the cap is checked under the class row lock.
func (s *Service) CreateInvite(ctx context.Context, userID uint64, classID string, expiresAt *int64) (*CreatedInvite, error) {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	exp := now.Add(DefaultInviteLifetime).UnixMilli()
	if expiresAt != nil {
		if exp = *expiresAt; exp <= now.UnixMilli() || exp > now.Add(MaxInviteLifetime).UnixMilli() {
			return nil, invalid("expires_at")
		}
	}
	if err := s.throttle(ctx, userID); err != nil {
		return nil, err
	}
	token, hash, err := newInviteToken()
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "generate invite token")
	}
	inv := Invite{ID: ulid.New(now), CreatedAt: now.UnixMilli(), ExpiresAt: exp, Active: true}
	if err := s.store.createInvite(ctx, userID, classID, &inv, hash, now.UnixMilli()); err != nil {
		return nil, err
	}
	return &CreatedInvite{Invite: inv, Token: token, Link: s.baseURL + joinPath + token}, nil
}

// Invites returns one page of the class's invites (admin), newest first, revoked and expired ones included, never a token.
func (s *Service) Invites(ctx context.Context, userID uint64, classID string, limit int, nextID string) ([]Invite, string, error) {
	c, err := s.Authorize(ctx, userID, classID, RoleAdmin)
	if err != nil {
		return nil, "", err
	}
	scope := inviteListScope(userID, c.ID)
	var after uint64
	if nextID != "" {
		if after, err = openPos(s.tokens, scope, nextID); err != nil {
			return nil, "", err
		}
	}
	out, next, err := s.store.invites(ctx, c.internalID, after, limit, s.now().UnixMilli())
	if err != nil {
		return nil, "", err
	}
	return out, sealPos(s.tokens, scope, next), nil
}

// RevokeInvite stops a link at once (admin); revoking it again succeeds. Members who joined with it stay. An invite id of another
// class is ErrInviteNotFound and nothing changes.
func (s *Service) RevokeInvite(ctx context.Context, userID uint64, classID, inviteID string) error {
	if _, err := s.Authorize(ctx, userID, classID, RoleAdmin); err != nil {
		return err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return err
	}
	return s.store.revokeInvite(ctx, userID, classID, inviteID, s.now().UnixMilli())
}

// LookupInvite is the public preview of a link: the class name and school for an active token, ErrInviteNotFound for every other
// token (unknown, malformed, expired, revoked: one answer, one code path). ip is the client address; both limiters fail open
// because they bound load, and a 256-bit token cannot be guessed.
func (s *Service) LookupInvite(ctx context.Context, ip, token string) (*InvitePreview, error) {
	if ok, wait, _ := s.lookupAll.Take(ctx, ip); !ok {
		return nil, tooMany(wait)
	}
	if ok, wait, _ := s.lookupMisses.Take(ctx, ip); !ok {
		return nil, tooMany(wait)
	}
	hash, valid := inviteHash(token)
	var inv *inviteRow
	if valid {
		var err error
		if inv, err = s.store.inviteByHash(ctx, hash); err != nil {
			return nil, err
		}
	}
	if inv == nil || !inv.active(s.now().UnixMilli()) {
		return nil, ErrInviteNotFound
	}
	_ = s.lookupMisses.Refund(ctx, ip) // a live link is not a guess
	return &InvitePreview{Name: inv.className, SchoolName: inv.schoolName}, nil
}

// AcceptInvite adds the caller to the class as a student (verified email). Idempotent: a caller who is already a member gets
// their membership back and keeps their role (an admin is never downgraded). An invalid token spends the caller's write budget
// like any other write, so guessing is bounded per user and per IP. A full class is ErrClassFull. Returns the class as class/get does.
func (s *Service) AcceptInvite(ctx context.Context, userID uint64, ip string, emailVerified bool, token string) (*Class, error) {
	if !emailVerified {
		return nil, ErrEmailNotVerified
	}
	if err := s.throttle(ctx, userID); err != nil {
		return nil, err
	}
	ok, wait, err := s.acceptIP.Take(ctx, ip)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, tooMany(wait)
	}
	hash, valid := inviteHash(token)
	if !valid {
		return nil, ErrInviteNotFound
	}
	now := s.now().UTC()
	return s.store.accept(ctx, userID, hash, ulid.New(now), now.UnixMilli())
}
