package table

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/kolide/launcher/pkg/osquery"
	qt "github.com/kolide/launcher/pkg/pb/querytarget"
	"github.com/osquery/osquery-go/plugin/table"

	"go.etcd.io/bbolt"
)

const TargetMembershipKey = "target_membership"
const targetMembershipTableName = "kolide_target_membership"

func TargetMembershipTable(db *bbolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("id"),
	}

	return table.NewPlugin(targetMembershipTableName, columns, generateTargetMembershipTable(db))
}

func generateTargetMembershipTable(db *bbolt.DB) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

		var targetRespBytes []byte
		if err := db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte(osquery.ServerProvidedDataBucket))
			targetRespBytes = b.Get([]byte(TargetMembershipKey))

			return nil
		}); err != nil {
			return nil, fmt.Errorf("fetching data: %w", err)
		}

		var cachedResp qt.GetTargetsResponse
		if err := proto.Unmarshal(targetRespBytes, &cachedResp); err != nil {
			return nil, fmt.Errorf("unmarshalling target resp: %w", err)
		}

		targets := cachedResp.GetTargets()

		results := make([]map[string]string, len(targets))
		for i, t := range targets {
			results[i] = map[string]string{"id": t.GetId()}
		}

		return results, nil
	}
}
