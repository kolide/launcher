package querytarget

import (
	"context"
	"time"

	"go.etcd.io/bbolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/table"
	qt "github.com/kolide/launcher/pkg/pb/querytarget"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type QueryTargetUpdater struct {
	logger       log.Logger
	db           *bbolt.DB
	targetClient qt.QueryTargetClient
}

func NewQueryTargeter(logger log.Logger, db *bbolt.DB, grpcConn *grpc.ClientConn) QueryTargetUpdater {
	return QueryTargetUpdater{
		logger:       logger,
		db:           db,
		targetClient: qt.NewQueryTargetClient(grpcConn),
	}
}

func (qtu *QueryTargetUpdater) updateTargetMemberships(ctx context.Context) error {
	nodeKey, err := osquery.NodeKeyFromDB(qtu.db)
	if err != nil {
		return errors.Wrap(err, "getting node key from db")
	}

	resp, err := qtu.targetClient.GetTargets(ctx, &qt.GetTargetsRequest{NodeKey: nodeKey})
	if err != nil {
		return errors.Wrap(err, "fetching target memberships")
	}

	targetRespBytes, err := proto.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshaling targets to bytes")
	}

	if err := qtu.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(osquery.ServerProvidedDataBucket))
		err := b.Put([]byte(table.TargetMembershipKey), targetRespBytes)

		return errors.Wrap(err, "updating target memberships")
	}); err != nil {
		return err
	}

	return nil
}

func (qtu *QueryTargetUpdater) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := qtu.updateTargetMemberships(ctx); err != nil {
				if status.Code(errors.Cause(err)) == codes.Unimplemented {
					level.Debug(qtu.logger).Log(
						"msg", "server does not implement GetTargets",
					)
				} else {
					level.Error(qtu.logger).Log(
						"msg", "updating kolide_target_membership data",
						"err", err,
					)
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
