package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

const maxRequestBody = 1 << 20

type problem struct {
	Error     string         `json:"error"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return domain.FieldError{Field: "body", Message: "must be valid JSON: " + err.Error()}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.FieldError{Field: "body", Message: "must contain one JSON value"}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeProblem(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, problem{Error: code, Message: message, Details: details})
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "the server could not complete the request"
	switch {
	case errors.Is(err, contextCanceled()):
		status, code, message = 499, "request_cancelled", "the request was cancelled"
	case errors.Is(err, domain.ErrValidation):
		status, code, message = http.StatusUnprocessableEntity, "validation_failed", err.Error()
	case errors.Is(err, domain.ErrUnauthenticated):
		status, code, message = http.StatusUnauthorized, "unauthenticated", "authentication failed"
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", "the operation is not permitted"
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "the requested resource was not found"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrLeaseLost):
		status, code, message = http.StatusConflict, "conflict", err.Error()
	case errors.Is(err, domain.ErrUnavailable):
		status, code, message = http.StatusServiceUnavailable, "unavailable", "a dependency is temporarily unavailable"
	}
	writeProblem(w, status, code, message, nil)
}

func contextCanceled() error {
	return context.Canceled
}
