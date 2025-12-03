package localserver

import (
	"log/slog"
	"net/http"
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

		defer func() {
			ls.slogger.Log(r.Context(), slog.LevelInfo,
				"request log",
				"path", r.URL.Path,
				"method", r.Method,
				"status", recorder.Status,
			)
		}()

		next.ServeHTTP(recorder, r)
	})
}
