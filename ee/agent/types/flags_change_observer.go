package types

import (
	"context"

	"github.com/kolide/launcher/ee/agent/flags/keys"
)

// FlagsChangeObserver is an interface to be notified of changes to flags.
//
//mockery:generate: true
//mockery:filename: flags_change_observer.go
//mockery:structname: FlagsChangeObserver
type FlagsChangeObserver interface {
	// FlagsChanged tells the observer that flag changes have occurred.
	FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey)
}
