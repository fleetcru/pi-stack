package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// Stable API error codes for clients (pi-desk). Keep human "error" for compatibility.
const (
	CodeUnauthorized          = "unauthorized"
	CodeForbidden             = "forbidden"
	CodeNotFound              = "not_found"
	CodeBadRequest            = "bad_request"
	CodeConflict              = "conflict"
	CodeWorkerHasActive       = "worker_has_active_sessions"
	CodeWorkerNotFound        = "worker_not_found"
	CodeSessionNotFound       = "session_not_found"
	CodeInvalidTicket         = "invalid_ticket"
	CodeTicketExpired         = "ticket_expired"
	CodeTicketUsed            = "ticket_used"
	CodeTicketSessionMismatch = "ticket_session_mismatch"
	CodeCapacityExceeded      = "capacity_exceeded"
	CodeInternal              = "internal_error"
	CodeBadGateway            = "bad_gateway"
	CodePathNotAllowed        = "path_not_allowed"
	CodeFileTooLarge          = "file_too_large"
	CodeNotAFile              = "not_a_file"
	CodeBinaryFile            = "binary_file"
	CodeOriginNotAllowed      = "origin_not_allowed"
	CodeWorkerHostNotAllowed  = "worker_host_not_allowed"
	CodeLocalWorkerImmutable  = "local_worker_immutable"
	CodeCapabilityUnsupported = "capability_unsupported"
)

type requestIDKey struct{}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte("fallback-request-id"))
	}
	return hex.EncodeToString(b[:])
}

func requestIDFrom(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return id
	}
	return r.Header.Get("X-Request-ID")
}

func contextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func writeErrorCode(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	payload := map[string]any{"error": msg, "code": code}
	if id := requestIDFrom(r); id != "" {
		payload["requestId"] = id
		w.Header().Set("X-Request-ID", id)
	}
	writeJSON(w, status, payload)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeErrorText(w, code, err.Error())
}

func writeErrorText(w http.ResponseWriter, code int, msg string) {
	// Compatibility path used by existing call sites without request context.
	// Prefer writeErrorCode from new handlers.
	statusCode := mapStatusToCode(code)
	writeJSON(w, code, map[string]any{"error": msg, "code": statusCode})
}

func mapStatusToCode(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	case http.StatusTooManyRequests:
		return CodeCapacityExceeded
	case http.StatusBadGateway:
		return CodeBadGateway
	case http.StatusInternalServerError:
		return CodeInternal
	default:
		return CodeBadRequest
	}
}
