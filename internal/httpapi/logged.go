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

		attributes := []any{
			"method", request.Method,
			"path", pathClass(request.URL.Path),
			"status", recorder.status,
			"duration_ms", duration.Milliseconds(),
		}
		switch {
		case recorder.status >= http.StatusInternalServerError:
			slog.Error("request failed", attributes...)
		case duration > slowRequest:
			slog.Warn("request slow", attributes...)
		case recorder.status == http.StatusUnauthorized || recorder.status == http.StatusForbidden:
			slog.Debug("request refused", attributes...)
		}
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

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
