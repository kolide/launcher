package menu

import (
	"context"
	"log/slog"
)

// Performs the OpenURL action
type actionOpenURL struct {
	URL string `json:"url"`
}

func (a actionOpenURL) Perform(m *menu) {
	if err := open(a.URL); err != nil {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"failed to perform action",
			"url", a.URL,
			"err", err,
		)
	}
}
