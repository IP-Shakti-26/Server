package respond

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// JSON writes a JSON response with the given status code and data payload.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Error writes a JSON error response. Optional details are treated as
// sequential key-value pairs appended to the response body.
// Example: Error(w, 400, "too short", "min_length", 20)
func Error(w http.ResponseWriter, status int, message string, details ...any) {
	body := map[string]any{
		"error": message,
	}
	for i := 0; i+1 < len(details); i += 2 {
		if key, ok := details[i].(string); ok {
			body[key] = details[i+1]
		}
	}
	JSON(w, status, body)
}

// NotFound is a convenience wrapper that returns a 404 with "<resource> not found".
func NotFound(w http.ResponseWriter, resource string) {
	Error(w, http.StatusNotFound, resource+" not found")
}

// InternalError logs the full error via slog and returns a generic 500 to
// the client. Internal error details are never exposed to callers.
func InternalError(w http.ResponseWriter, err error, logger *slog.Logger) {
	logger.Error("internal server error", "error", err)
	Error(w, http.StatusInternalServerError, "internal server error")
}
