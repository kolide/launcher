package runner

import (
	"log/slog"
	"strings"
)

type errFilter struct {
	matchStrings []string
}

func (e *errFilter) filter(err error) slog.Level {
	for _, matchString := range e.matchStrings {
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(matchString)) {
			return slog.LevelWarn
		}
	}

	return slog.LevelError
}
