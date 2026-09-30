// Package httpx provides the HTTP foundation shared by every module: the error contract,
// JSON response writing and the middleware chain.
//
// JSON strategy: stdlib encoding/json, encoding straight to the ResponseWriter. It is the
// baseline measured in docs/PERFORMANCE_BASELINE.md; a faster encoder is only adopted if a
// benchmark on a real endpoint justifies the extra dependency.
package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSON writes v as JSON with the given status. Encoding failures after the header is sent
// cannot change the response, so they are logged rather than returned.
func WriteJSON(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.WarnContext(ctx, "writing JSON response failed", slog.Any("error", err))
	}
}
