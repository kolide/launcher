package main

import (
	"context"

	"go.etcd.io/bbolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/actor"
	"github.com/kolide/launcher/pkg/querytarget"
	"google.golang.org/grpc"
)

func createQueryTargetUpdater(logger log.Logger, db *bbolt.DB, grpcConn *grpc.ClientConn) *actor.Actor {
	ctx, cancel := context.WithCancel(context.Background())

	updater := querytarget.NewQueryTargeter(logger, db, grpcConn)

	return &actor.Actor{
		Execute: func() error {
			level.Info(logger).Log("msg", "query target updater started")
			updater.Run(ctx)
			return nil
		},
		Interrupt: func(err error) {
			level.Info(logger).Log("msg", "query target updater interrupted")
			cancel()
		},
	}
}
