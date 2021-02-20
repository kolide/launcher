// +build windows

package main

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/launcher"
	"go.etcd.io/bbolt"
)

// createControl creates a no-op actor, as the control server isn't
// yet supported on windows.
func createControl(ctx context.Context, db *bbolt.DB, logger log.Logger, opts *launcher.Options) (*actor.Actor, error) {
	level.Info(logger).Log("msg", "Cannot create control channel for windows, ignoring")

	return nil, nil
}
