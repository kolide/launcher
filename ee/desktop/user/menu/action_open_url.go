package menu

import (
	"github.com/go-kit/kit/log/level"
)

// Performs the OpenURL action
type actionOpenURL struct {
	URL string `json:"url"`
}

func (a actionOpenURL) Perform(m *menu) {
	if err := open(a.URL); err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to perform action",
			"URL", a.URL,
			"err", err)
	}
}
