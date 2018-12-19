package main

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/querytarget"
	"google.golang.org/grpc"
)

func createQueryTargetUpdater(ctx context.Context, logger log.Logger, db *bolt.DB, grpcConn *grpc.ClientConn) *actor.Actor {
	updater := querytarget.NewQueryTargeter(logger, db, grpcConn)

	return &actor.Actor{
		Execute: func() error {
			level.Info(logger).Log("msg", "query target updater started")
			updater.Start(ctx)
			return nil
		},
		Interrupt: func(err error) {
			level.Info(logger).Log("msg", "query target updater interrupted", "err", err)
			updater.Stop()
		},
	}
}
