package class

import (
	"strconv"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Tokens makes and reads the opaque next_id tokens; *httpx.Cursors implements it.
type Tokens interface {
	Seal(scope, pos string) string
	Open(scope, token string) (pos string, err error)
}

// The position of a page is the internal id of the last row read, sealed (AES-GCM) so a client can neither read nor change
// it. The scope binds the token to one list of one caller: a token from another list, class or user is ERROR_PARAM.
func classListScope(userID uint64) string { return "class/get-list:" + strconv.FormatUint(userID, 10) }

func memberListScope(userID uint64, classID string) string {
	return "class-member/get-list:" + classID + ":" + strconv.FormatUint(userID, 10)
}

func inviteListScope(userID uint64, classID string) string {
	return "class-invite/get-list:" + classID + ":" + strconv.FormatUint(userID, 10)
}

// sealPos returns the token for the page that follows row ("" when row is 0: no further page).
func sealPos(t Tokens, scope string, row uint64) string {
	if row == 0 {
		return ""
	}
	return t.Seal(scope, strconv.FormatUint(row, 10))
}

// openPos returns the row in token; any malformed, tampered or foreign token is ERROR_PARAM.
func openPos(t Tokens, scope, token string) (uint64, error) {
	pos, err := t.Open(scope, token)
	if err != nil {
		return 0, err
	}
	row, perr := strconv.ParseUint(pos, 10, 64)
	if perr != nil || row == 0 {
		return 0, apperr.New(apperr.Param, "next_id is not a valid page token")
	}
	return row, nil
}
