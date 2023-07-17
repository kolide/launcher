package menu

import (
	"encoding/json"
	"fmt"
)

// actionTypes are named identifiers
type actionType string

const (
	DoNothing actionType = "" // Omitted action implies do nothing
	OpenURL              = "open-url"
	Flare                = "flare"
)

// Action encapsulates what action should be performed when a menu item is invoked
type Action struct {
	Type      actionType      `json:"type"`
	Action    json.RawMessage `json:"action,omitempty"`
	Performer ActionPerformer `json:"-"`
}

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

	// The type and action is easily unmarshaled
	a.Type = action.Type
	a.Action = action.Action

	// Based on the type, determine the appropriate performer to unmarshal & instantiate
	switch a.Type {
	case DoNothing:
	case OpenURL:
		openURL := actionOpenURL{}
		if err := json.Unmarshal(a.Action, &openURL); err != nil {
			return fmt.Errorf("failed to unmarshal ActionOpenURL: %w", err)
		}
		a.Performer = openURL
	case Flare:
		a.Performer = actionFlare{}
	default:
		// Silently ignore unrecognized actions because:
		// 1. We don't have a logger reference here
		// 2. As new actions are added, we don't want older versions of launcher to fail to unmarshal menus
	}

	return nil
}
