package yearbook

import (
	"strconv"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

const (
	// A cursor time outside years 1..9999 is a tampered token, not a position in anyone's list.
	minCursorMs = -62135596800000 // 0001-01-01T00:00:00Z
	maxCursorMs = 253402300799999 // 9999-12-31T23:59:59.999Z

	publicIDLen = 26 // a ULID
)

// Tokens makes and reads the opaque next_id tokens; *httpx.Cursors implements it.
type Tokens interface {
	Seal(scope, pos string) string
	Open(scope, token string) (pos string, err error)
}

// cursorScope binds a token to the caller's list: a token taken from another user's list is refused, so a client can
// neither read nor change the position inside it (the position is "<updated_at Unix ms>.<book id>", sealed).
func cursorScope(ownerID uint64) string {
	return "yearbook/get-list:" + strconv.FormatUint(ownerID, 10)
}

func sealCursor(t Tokens, ownerID uint64, c *cursor) string {
	if c == nil {
		return ""
	}
	return t.Seal(cursorScope(ownerID), strconv.FormatInt(c.updatedAt, 10)+"."+c.publicID)
}

// openCursor returns the position in token; any malformed, tampered or foreign token is ERROR_PARAM.
func openCursor(t Tokens, ownerID uint64, token string) (*cursor, error) {
	pos, err := t.Open(cursorScope(ownerID), token)
	if err != nil {
		return nil, err
	}
	ms, id, ok := strings.Cut(pos, ".")
	n, perr := strconv.ParseInt(ms, 10, 64)
	if !ok || perr != nil || len(id) != publicIDLen || n < minCursorMs || n > maxCursorMs {
		return nil, apperr.New(apperr.Param, "next_id is not a valid page token")
	}
	return &cursor{n, id}, nil
}
