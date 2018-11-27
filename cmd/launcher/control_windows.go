// +build windows

package main

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/actor"
	"github.com/pkg/errors"
)

func createControl(ctx context.Context, db *bolt.DB, logger log.Logger, opts *options) (*actor.Actor, error) {
	return nil, errors.New("control is not supported for windows")
}
