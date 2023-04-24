package types

import "github.com/kolide/launcher/pkg/agent/flags/keys"

// FlagsChangeObserver is an interface to be notified of changes to flags.
type FlagsChangeObserver interface {
	// FlagsChanged tells the observer that flag changes have occurred.
	FlagsChanged(flagKeys ...keys.FlagKey)
}
