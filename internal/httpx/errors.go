package httpx

import (
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
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

// WriteError writes the shared envelope {"error":{"code","message","request_id"}}.
// code is a stable snake_case contract; message is English text for logs.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	WriteJSON(w, status, errorEnvelope{ErrorBody{Code: code, Message: msg, RequestID: RequestIDFrom(r.Context())}})
}

// WriteBodyError reports a failed request-body read: 413 payload_too_large when the
// body cap was hit, 400 invalid_body otherwise.
func WriteBodyError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large")
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
