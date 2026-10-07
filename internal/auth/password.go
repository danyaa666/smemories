package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// ErrBusy means every hashing slot stayed taken for the whole wait; the API answers 503 busy.
var ErrBusy = errors.New("auth: too many password hashes in flight")

const (
	saltLen = 16
	keyLen  = 32
	// Upper bounds for parameters read back from a stored hash, so a corrupted row cannot
	// make one login allocate gigabytes.
	maxStoredMemoryKiB = 1 << 20
	maxStoredTime      = 100
)

// HashParams are the argon2id cost parameters (config: SMEM_AUTH_ARGON_*).
type HashParams struct {
	MemoryKiB   uint32
	Time        uint32
	Parallelism uint8
}

// Hasher hashes and verifies passwords, running at most cap(slots) argon2id computations
// at once (each needs MemoryKiB of RAM).
type Hasher struct {
	params HashParams
	slots  chan struct{}
	wait   time.Duration // how long to wait for a slot before ErrBusy
}

// NewHasher returns a Hasher allowing maxConcurrent simultaneous hashes.
func NewHasher(p HashParams, maxConcurrent int, wait time.Duration) *Hasher {
	return &Hasher{params: p, slots: make(chan struct{}, maxConcurrent), wait: wait}
}

func (h *Hasher) acquire(ctx context.Context) error {
	t := time.NewTimer(h.wait)
	defer t.Stop()
	select {
	case h.slots <- struct{}{}:
		return nil
	case <-t.C:
		return ErrBusy
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Hasher) release() { <-h.slots }

// Hash returns the PHC string `$argon2id$v=19$m=..,t=..,p=..$salt$key` for password.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, h.params.Time, h.params.MemoryKiB, h.params.Parallelism, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		h.params.MemoryKiB, h.params.Time, h.params.Parallelism, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify checks password against a PHC string using the parameters stored in it
// (so tuning the config never locks existing users out). The comparison is constant-time.
func (h *Hasher) Verify(ctx context.Context, password, phc string) (bool, error) {
	var (
		version int
		p       HashParams
	)
	parts := strings.Split(phc, "$") // "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("auth: stored hash is not argon2id PHC")
	}
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("auth: unsupported argon2 version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.MemoryKiB, &p.Time, &p.Parallelism); err != nil ||
		p.MemoryKiB == 0 || p.MemoryKiB > maxStoredMemoryKiB || p.Time == 0 || p.Time > maxStoredTime || p.Parallelism == 0 {
		return false, errors.New("auth: bad argon2 parameters")
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[4])
	want, err2 := enc.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(salt) == 0 || len(want) == 0 {
		return false, errors.New("auth: bad argon2 encoding")
	}
	if err := h.acquire(ctx); err != nil {
		return false, err
	}
	defer h.release()
	got := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
