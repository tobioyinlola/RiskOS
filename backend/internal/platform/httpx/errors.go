package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

// Stable error codes of the public error contract (spec §7). Codes are part of the API; never
// rename one once released.
const (
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeForbidden          = "FORBIDDEN"
	CodeTenantSuspended    = "TENANT_SUSPENDED"
	CodeConflict           = "CONFLICT"
	CodeNotFound           = "NOT_FOUND"
	CodeMethodNotAllowed   = "METHOD_NOT_ALLOWED"
	CodePayloadTooLarge    = "PAYLOAD_TOO_LARGE"
	CodeTimeout            = "TIMEOUT"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeInternal           = "INTERNAL_ERROR"
)

// Error is an error that maps to a client-safe HTTP response. Message is shown to clients and must
// never contain internals; Cause is logged server-side only.
type Error struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// NewError constructs an Error.
func NewError(status int, code, message string, cause error) *Error {
	return &Error{Status: status, Code: code, Message: message, Cause: cause}
}

// InvalidRequest is a 400 with a client-safe message.
func InvalidRequest(message string, cause error) *Error {
	return NewError(http.StatusBadRequest, CodeInvalidRequest, message, cause)
}

// NotFound is the uniform 404. Resources of other tenants or environments use it too, so their
// existence is never revealed.
func NotFound(cause error) *Error {
	return NewError(http.StatusNotFound, CodeNotFound, "The requested resource does not exist.", cause)
}

// Conflict is a 409 with a client-safe message.
func Conflict(message string, cause error) *Error {
	return NewError(http.StatusConflict, CodeConflict, message, cause)
}

// toError maps any error to a client-safe Error. Unknown errors become 500 INTERNAL_ERROR so
// that no internal detail can leak through the error contract.
func toError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, authz.ErrForbidden):
		return NewError(http.StatusForbidden, CodeForbidden, "The credential lacks the permission required for this operation.", err)
	case errors.As(err, &tooLarge):
		return NewError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "The request body is too large.", err)
	case errors.Is(err, context.DeadlineExceeded):
		return NewError(http.StatusServiceUnavailable, CodeTimeout, "The request timed out.", err)
	default:
		return NewError(http.StatusInternalServerError, CodeInternal, "An internal error occurred.", err)
	}
}

// WriteError writes err in the standard error contract and logs it. 5xx errors are logged at
// error level with their cause; 4xx at debug, since they describe client behavior.
func WriteError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	e := toError(err)
	ctx := r.Context()
	if e.Status >= http.StatusInternalServerError {
		logger.ErrorContext(ctx, "request failed", slog.String("code", e.Code), slog.Int("status", e.Status), slog.Any("error", err))
	} else {
		logger.DebugContext(ctx, "request rejected", slog.String("code", e.Code), slog.Int("status", e.Status), slog.Any("error", err))
	}
	WriteJSON(ctx, w, logger, e.Status, apigen.ErrorResponse{Error: apigen.ErrorDetail{
		Code:      e.Code,
		Message:   e.Message,
		RequestId: requestctx.RequestID(ctx),
	}})
}

// NotFoundHandler returns 404 in the error contract for unknown routes.
func NotFoundHandler(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, logger, NewError(http.StatusNotFound, CodeNotFound, "The requested resource does not exist.", nil))
	}
}

// MethodNotAllowedHandler returns 405 in the error contract.
func MethodNotAllowedHandler(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, logger, NewError(http.StatusMethodNotAllowed, CodeMethodNotAllowed, "The method is not allowed for this resource.", nil))
	}
}
