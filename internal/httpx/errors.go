package httpx

import (
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

// WriteError writes the v2 error envelope (docs/api-contract.md): code is the ERROR_* name, msg English text for logs.
// It is for the middleware and the router fallbacks, which hold a status and a name rather than an error;
// handlers use Fail.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	writeV2Error(w, r, status, code, msg)
}

// WriteBodyError reports a failed request-body read: 413 ERROR_TOO_LARGE when the
// body cap was hit, 408 ERROR_REQUEST_TIMEOUT on a read deadline, 400 ERROR_PARAM otherwise.
func WriteBodyError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteError(w, r, http.StatusRequestEntityTooLarge, apperr.TooLarge.Name, "request body too large")
		return
	}
	if errors.Is(err, os.ErrDeadlineExceeded) { // a read deadline, e.g. IdleBody: the client stopped sending
		WriteError(w, r, http.StatusRequestTimeout, "ERROR_REQUEST_TIMEOUT", "the request body arrived too slowly")
		return
	}
	WriteError(w, r, http.StatusBadRequest, apperr.Param.Name, "request body could not be read")
}

// DecodeJSON reads exactly one JSON object from the body into v. On failure it writes the error
// response itself and returns false: 413 ERROR_TOO_LARGE or 400 ERROR_PARAM (also for a field v does not
// declare).
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if err == nil {
		if _, e := dec.Token(); e != io.EOF { // anything after the object is an error
			err = cmp.Or(e, apperr.New(apperr.Param, "trailing data"))
		}
	}
	if err == nil {
		return true
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		WriteError(w, r, http.StatusBadRequest, apperr.Param.Name, "request body has an unknown field")
		return false
	}
	WriteBodyError(w, r, err)
	return false
}
