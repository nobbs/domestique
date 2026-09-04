package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// slowRequest is how long an answer may take before it is worth a log line.
const slowRequest = 2 * time.Second

// logged reports failed, slow and refused answers. Every other request passes
// in silence: the path is cut to its first two segments so no route ID reaches
// a log, and the query string is never read.
func (h *Handler) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := h.now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		duration := h.now().Sub(start)

		var level slog.Level
		var message string
		switch {
		case recorder.status >= http.StatusInternalServerError:
			level, message = slog.LevelError, "request failed"
		case duration > slowRequest:
			level, message = slog.LevelWarn, "request slow"
		case recorder.status == http.StatusUnauthorized || recorder.status == http.StatusForbidden:
			level, message = slog.LevelDebug, "request refused"
		default:
			return
		}
		slog.Log(request.Context(), level, message,
			"method", request.Method,
			"path", pathClass(request.URL.Path),
			"status", recorder.status,
			"duration_ms", duration.Milliseconds(),
		)
	})
}

// pathClass keeps the leading two segments of a path: "/v1/routes/abc/geometry"
// becomes "/v1/routes".
func pathClass(path string) string {
	segments := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	if len(segments) > 2 {
		segments = segments[:2]
	}

	return "/" + strings.Join(segments, "/")
}

// statusRecorder keeps the status net/http honours: the first one written.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.written {
		r.status, r.written = status, true
	}
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
