package menu

import (
	"context"
	"log/slog"
)

// actionFlare performs the launcher flare action. This will make more sense once flare is uploading
type actionFlare struct {
	URL string `json:"url"`
}

func (a actionFlare) Perform(m *menu) {
	if err := runFlare(); err != nil {
		m.slogger.Log(context.TODO(), slog.LevelError,
			"error creating flare",
			"err", err,
		)
	}
}

func runFlare() error {
	// TODO: can't create a flare without knapsack, and ideally, being root. This probably is going to be easiest if we use
	// IPC to send the request for flare back to the parent instead of running it inline here. But it's a bit TBD. Caution
	// is merited -- the user controls menu, so flare should not expose anything untoward.
	return nil
}
