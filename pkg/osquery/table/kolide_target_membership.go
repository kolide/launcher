package table

import (
	"context"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/pb/kt"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const targetMembershipKey = "target_membership"

type TargetMembership struct {
	logger       log.Logger
	db           *bolt.DB
	targetClient kt.KTargetClient
}

func NewTargetMembership(logger log.Logger, db *bolt.DB, targetClient kt.KTargetClient) TargetMembership {
	return TargetMembership{
		logger:       logger,
		db:           db,
		targetClient: targetClient,
	}
}

func (tm *TargetMembership) Plugin() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("id"),
	}

	return table.NewPlugin("kolide_target_membership", columns, tm.generate())
}

func (tm *TargetMembership) generate() table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

		targets, err := tm.GetTargetMembership(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "getting target membership")
		}

		results := make([]map[string]string, len(targets))
		for i, t := range targets {
			results[i] = map[string]string{"id": t.GetId()}
		}

		return results, nil
	}
}

func (tm *TargetMembership) Run(ctx context.Context) error {
	timeChan := time.NewTicker(30 * time.Second).C

	for {
		select {
		case <-timeChan:
			if err := tm.UpdateTargetMemberships(ctx); err != nil {
				level.Debug(tm.logger).Log(
					"msg", "updating target membership",
					"err", err,
				)
			}
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}

func (tm *TargetMembership) GetTargetMembership(ctx context.Context) ([]*kt.Target, error) {
	var targetRespBytes []byte
	if err := tm.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(osquery.ServerProvidedDataBucket))
		targetRespBytes = b.Get([]byte(targetMembershipKey))

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "fetching data")
	}

	var cachedResp kt.GetTargetsResponse
	if err := proto.Unmarshal(targetRespBytes, &cachedResp); err != nil {
		return nil, errors.Wrap(err, "unmarshaling target resp")
	}

	return cachedResp.GetTargets(), nil
}

func (tm *TargetMembership) UpdateTargetMemberships(ctx context.Context) error {
	nodeKey, err := osquery.NodeKeyFromDB(tm.db)
	if err != nil {
		return errors.Wrap(err, "getting node key from db")
	}

	resp, err := tm.targetClient.GetTargets(ctx, &kt.GetTargetsRequest{NodeKey: nodeKey})
	if err != nil {
		return errors.Wrap(err, "fetching target memberships")
	}

	targetRespBytes, err := proto.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshaling targets to bytes")
	}

	if err := tm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(osquery.ServerProvidedDataBucket))
		err := b.Put([]byte(targetMembershipKey), targetRespBytes)

		return errors.Wrap(err, "updating target memberships")
	}); err != nil {
		return err
	}

	return nil
}
