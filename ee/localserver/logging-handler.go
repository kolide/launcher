package localserver

import (
	"log/slog"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	Status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}

func (ls *localServer) requestLoggingHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, Status: 200}

		defer func(begin time.Time) {
			ls.slogger.Log(r.Context(), slog.LevelInfo,
				"request log",
				"path", r.URL.Path,
				"method", r.Method,
				"status", recorder.Status,
				"took", time.Since(begin),
			)
		}(time.Now())

		next.ServeHTTP(recorder, r)
	})
}
