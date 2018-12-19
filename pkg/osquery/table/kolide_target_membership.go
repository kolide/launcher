package table

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/gogo/protobuf/proto"
	"github.com/kolide/launcher/pkg/osquery"
	qt "github.com/kolide/launcher/pkg/pb/querytarget"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const TargetMembershipKey = "target_membership"
const targetMembershipTableName = "kolide_target_membership"

func TargetMembershipTable(db *bolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("id"),
	}

	return table.NewPlugin(targetMembershipTableName, columns, generateTargetMembershipTable(db))
}

func generateTargetMembershipTable(db *bolt.DB) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

		var targetRespBytes []byte
		if err := db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(osquery.ServerProvidedDataBucket))
			targetRespBytes = b.Get([]byte(TargetMembershipKey))

			return nil
		}); err != nil {
			return nil, errors.Wrap(err, "fetching data")
		}

		var cachedResp qt.GetTargetsResponse
		if err := proto.Unmarshal(targetRespBytes, &cachedResp); err != nil {
			return nil, errors.Wrap(err, "unmarshaling target resp")
		}

		targets := cachedResp.GetTargets()

		results := make([]map[string]string, len(targets))
		for i, t := range targets {
			results[i] = map[string]string{"id": t.GetId()}
		}

		return results, nil
	}
}
