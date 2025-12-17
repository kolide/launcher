package osquerypublisher

import (
	"log/slog"
	"net/http"
)

type (
	PublisherHTTPClient interface {
		Do(req *http.Request) (*http.Response, error)
	}
)

// levelForError returns slog.LevelWarn if err != nil, else slog.LevelDebug
func levelForError(err error) slog.Level {
	if err != nil {
		return slog.LevelWarn
	}
	return slog.LevelDebug
}
