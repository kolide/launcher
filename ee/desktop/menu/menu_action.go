package menu

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/go-kit/kit/log/level"
)

// actionTypes are named identifiers
type actionType string

const (
	DoNothing   actionType = "" // Omitted action implies do nothing
	OpenURL                = "open-url"
	RefreshMenu            = "refresh-menu"
)

// Action encapsulates what action should be performed when a menu item is invoked
type Action struct {
	Type      actionType      `json:"type"`
	Action    json.RawMessage `json:"action"`
	Performer ActionPerformer
}

// Performs the OpenURL action
type actionOpenURL struct {
	URL string `json:"url,omitempty"`
}

// Performs the RefreshMenu action
type refreshMenuAction struct{}

// ActionPerformer is an interface for performing actions in response to menu events
type ActionPerformer interface {
	// Perform executes the action
	Perform(m *menu)
}

// Used to avoid recursion in UnmarshalJSON
type action Action

func (a *Action) UnmarshalJSON(data []byte) error {
	action := action{}
	if err := json.Unmarshal(data, &action); err != nil {
		return fmt.Errorf("failed to unmarshal Action: %w", err)
	}
	switch action.Type {
	case DoNothing:
	case OpenURL:
		openURL := actionOpenURL{}
		if err := json.Unmarshal(action.Action, &openURL); err != nil {
			return fmt.Errorf("failed to unmarshal actionOpenURL: %w", err)
		}
		a.Performer = openURL
	case RefreshMenu:
		refreshMenu := refreshMenuAction{}
		if err := json.Unmarshal(action.Action, &refreshMenu); err != nil {
			return fmt.Errorf("failed to unmarshal refreshMenu: %w", err)
		}
		a.Performer = refreshMenu
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}

	return nil
}

func (a actionOpenURL) Perform(m *menu) {
	if err := open(a.URL); err != nil {
		level.Error(m.logger).Log(
			"msg", "failed to perform action",
			"URL", a.URL,
			"err", err)
	}
}

// open opens the specified URL in the default browser of the user
// See https://stackoverflow.com/a/39324149/1705598
func open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "/usr/bin/open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

func (a refreshMenuAction) Perform(m *menu) {
	m.Build()
}
