package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/redis"
)

const (
	purposeVerify = "verify"
	purposeReset  = "reset"

	verifyCodeTTL = 30 * time.Minute
	resetCodeTTL  = 15 * time.Minute
	// A code's hash stays in Redis this long after it expired, only so the student hears
	// code_expired instead of invalid_code. It can no longer be accepted.
	expiredGrace = time.Hour

	codeDigits      = 6
	maxCodeAttempts = 5

	// MinOTPKeyBytes is the shortest accepted SMEM_OTP_KEY.
	MinOTPKeyBytes = 32
)

// The outcomes of a code check; the handler maps them to 400 invalid_code, code_expired and
// code_locked.
var (
	ErrInvalidCode = apperr.New(apperr.Param, "invalid code")
	ErrCodeExpired = apperr.New(apperr.Param, "code expired")
	ErrCodeLocked  = apperr.New(apperr.Param, "too many wrong attempts on this code")
)

// Results of the check script.
const (
	resNone    = 0 // no code stored
	resOK      = 1 // right code, now deleted
	resWrong   = 2 // wrong code, attempt counted
	resLocked  = 3 // 5 wrong attempts used
	resExpired = 4 // lifetime over
)

// issueScript stores the HMAC of a new code (replacing any older one, attempts back to 0).
// KEYS[1] key; ARGV: hmac, expiry ms (unix), time to keep the key in ms.
const issueScript = `
redis.call('DEL', KEYS[1])
redis.call('HSET', KEYS[1], 'h', ARGV[1], 'a', 0, 'x', ARGV[2])
redis.call('PEXPIRE', KEYS[1], ARGV[3])
return 1`

// checkScript compares a guess and counts the attempt in one step, so parallel guesses can never
// exceed the limit. A locked code stays (until its Redis expiry) so later tries hear code_locked.
// KEYS[1] key; ARGV: hmac of the guess, now ms (unix), max attempts.
const checkScript = `
local v = redis.call('HMGET', KEYS[1], 'h', 'a', 'x')
if not v[1] then return 0 end
if tonumber(v[3]) <= tonumber(ARGV[2]) then return 4 end
local max = tonumber(ARGV[3])
if tonumber(v[2]) >= max then return 3 end
local g, h = ARGV[1], v[1]
local d = 0
if #g ~= #h then d = 1 end
for i = 1, math.min(#g, #h) do d = bit.bor(d, bit.bxor(g:byte(i), h:byte(i))) end
if d == 0 then
  redis.call('DEL', KEYS[1])
  return 1
end
if redis.call('HINCRBY', KEYS[1], 'a', 1) >= max then return 3 end
return 2`

// Codes issues and checks the 6-digit email codes (docs/auth-otp.md). All state lives in Redis,
// one hash per (purpose, user): {h: HMAC of the code, a: wrong attempts, x: expiry}.
type Codes struct {
	rc    *redis.Client
	key   []byte
	issue *goredis.Script
	check *goredis.Script
	fixed string // DEV-SHORTCUT(otp): code accepted for every check; "" = off
}

// NewCodes loads the scripts. key is SMEM_OTP_KEY. fixedOTP is SMEM_DEV_FIXED_OTP and is refused
// unless env is dev or test.
func NewCodes(ctx context.Context, rc *redis.Client, env string, key []byte, fixedOTP string) (*Codes, error) {
	if len(key) < MinOTPKeyBytes {
		return nil, apperr.New(apperr.Internal, "auth: OTP key too short")
	}
	if fixedOTP != "" && env != "dev" && env != "test" { // DEV-SHORTCUT(otp)
		return nil, apperr.New(apperr.Internal, "auth: SMEM_DEV_FIXED_OTP is only allowed when SMEM_ENV is dev or test") // DEV-SHORTCUT(otp)
	} // DEV-SHORTCUT(otp)
	is, err := rc.LoadScript(ctx, issueScript)
	if err != nil {
		return nil, err
	}
	cs, err := rc.LoadScript(ctx, checkScript)
	if err != nil {
		return nil, err
	}
	return &Codes{rc: rc, key: key, issue: is, check: cs, fixed: fixedOTP}, nil // DEV-SHORTCUT(otp)
}

func codeTTL(purpose string) time.Duration {
	if purpose == purposeReset {
		return resetCodeTTL
	}
	return verifyCodeTTL
}

func (c *Codes) redisKey(purpose string, uid uint64) string {
	return c.rc.Key("otp", purpose, strconv.FormatUint(uid, 10))
}

// mac is HMAC-SHA256(key, purpose | user id | code) in hex.
func (c *Codes) mac(purpose string, uid uint64, code string) string {
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte(purpose + "|" + strconv.FormatUint(uid, 10) + "|" + code))
	return hex.EncodeToString(m.Sum(nil))
}

// newCode draws 6 decimal digits uniformly (crypto/rand.Int rejects out-of-range draws), leading zeros kept.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	s := strconv.Itoa(int(n.Int64()))
	return "000000"[len(s):] + s, nil
}

func validCodeFormat(code string) bool {
	if len(code) != codeDigits {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

// Issue stores a new code for the user (the old one of that purpose dies) and returns it. Redis
// down is redis.ErrUnavailable.
func (c *Codes) Issue(ctx context.Context, purpose string, uid uint64, now time.Time) (string, error) {
	code, err := newCode()
	if err != nil {
		return "", apperr.Wrap(apperr.Internal, err, "draw code")
	}
	ttl := codeTTL(purpose)
	err = c.issue.Run(ctx, c.rc.Client(), []string{c.redisKey(purpose, uid)},
		c.mac(purpose, uid, code), now.Add(ttl).UnixMilli(), (ttl + expiredGrace).Milliseconds()).Err()
	if err = redis.Classify(err); err != nil {
		return "", err
	}
	return code, nil
}

// Check verifies code for an existing user and consumes it on success. Wrong, missing and
// malformed codes are ErrInvalidCode; also ErrCodeExpired, ErrCodeLocked, redis.ErrUnavailable.
func (c *Codes) Check(ctx context.Context, purpose string, uid uint64, code string, now time.Time) error {
	if c.fixed != "" && subtle.ConstantTimeCompare([]byte(code), []byte(c.fixed)) == 1 { // DEV-SHORTCUT(otp)
		return nil // DEV-SHORTCUT(otp): accepted for any existing user, no attempt counted
	} // DEV-SHORTCUT(otp)
	return c.verify(ctx, purpose, uid, code, now)
}

// Decoy does the work of Check for an address that has no account (user id 0 never has a code),
// so unknown and known addresses cost the same. It always fails: ErrInvalidCode, or
// redis.ErrUnavailable like Check would.
func (c *Codes) Decoy(ctx context.Context, purpose, code string, now time.Time) error {
	if err := c.verify(ctx, purpose, 0, code, now); errors.Is(err, redis.ErrUnavailable) {
		return err
	}
	return ErrInvalidCode
}

func (c *Codes) verify(ctx context.Context, purpose string, uid uint64, code string, now time.Time) error {
	if !validCodeFormat(code) {
		return ErrInvalidCode
	}
	res, err := c.check.Run(ctx, c.rc.Client(), []string{c.redisKey(purpose, uid)},
		c.mac(purpose, uid, code), now.UnixMilli(), maxCodeAttempts).Int()
	if err = redis.Classify(err); err != nil {
		return err
	}
	switch res {
	case resOK:
		return nil
	case resLocked:
		return ErrCodeLocked
	case resExpired:
		return ErrCodeExpired
	default: // resNone, resWrong
		return ErrInvalidCode
	}
}
