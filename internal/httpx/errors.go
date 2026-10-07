package httpx

import (
	"errors"
	"net/http"
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
