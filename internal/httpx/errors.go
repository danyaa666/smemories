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

// ErrorBody is the inner object of the shared error envelope.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// WriteError writes the shared error envelope. code is a stable snake_case contract and
// message is English text for logs.
//
// TRANSITION(E10): delete in T-072. Requests under /api/ get the v2 body
// {"status":"ERROR_...","error_message","request_id"} (docs/api-contract.md) with the legacy code
// mapped by v2Code; every other path keeps {"error":{"code","message","request_id"}}.
// This lets Recover, BodyLimit, DecodeJSON, RequireUser and the router fallbacks serve both
// generations while domains move to /api one at a time.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	if isV2(r) {
		writeV2Error(w, r, status, v2Code(code), msg)
		return
	}
	WriteJSON(w, status, errorEnvelope{ErrorBody{Code: code, Message: msg, RequestID: RequestIDFrom(r.Context())}})
}

// v2Prefix is the path prefix of the v2 contract.
const v2Prefix = "/api/"

func isV2(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, v2Prefix) }

// v2Code maps a legacy snake_case code to its v2 name: ERROR_ + upper case, except the
// codes the contract merged (docs/api-contract.md section 3).
func v2Code(code string) string {
	switch code {
	case "internal_error":
		return apperr.Internal.Name
	case "invalid_body", "unknown_field":
		return apperr.Param.Name
	case "payload_too_large":
		return apperr.TooLarge.Name
	case "unauthenticated":
		return apperr.Unauthorized.Name
	}
	return "ERROR_" + strings.ToUpper(code)
}

// WriteBodyError reports a failed request-body read: 413 payload_too_large when the
// body cap was hit, 408 request_timeout on a read deadline, 400 invalid_body otherwise.
func WriteBodyError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large")
		return
	}
	if errors.Is(err, os.ErrDeadlineExceeded) { // a read deadline, e.g. IdleBody: the client stopped sending
		WriteError(w, r, http.StatusRequestTimeout, "request_timeout", "the request body arrived too slowly")
		return
	}
	WriteError(w, r, http.StatusBadRequest, "invalid_body", "request body could not be read")
}

// DecodeJSON reads exactly one JSON object from the body into v. On failure it writes the error
// response itself and returns false: 413 payload_too_large, 400 unknown_field (only when strict
// and the body has a field v does not declare) or 400 invalid_body.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any, strict bool) bool {
	dec := json.NewDecoder(r.Body)
	if strict {
		dec.DisallowUnknownFields()
	}
	err := dec.Decode(v)
	if err == nil {
		if _, e := dec.Token(); e != io.EOF { // anything after the object is an error
			err = cmp.Or(e, errors.New("trailing data"))
		}
	}
	if err == nil {
		return true
	}
	if strict && strings.HasPrefix(err.Error(), "json: unknown field ") {
		WriteError(w, r, http.StatusBadRequest, "unknown_field", "request body has an unknown field")
		return false
	}
	WriteBodyError(w, r, err)
	return false
}
