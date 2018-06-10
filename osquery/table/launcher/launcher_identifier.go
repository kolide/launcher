package launcher

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/google/uuid"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func LauncherIdentifierTable(db *bolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("identifier"),
	}
	return table.NewPlugin("kolide_launcher_identifier", columns, generateLauncherIdentifier(db))
}

func generateLauncherIdentifier(db *bolt.DB) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		identifier, err := getIdentifierFromDB(db)
		if err != nil {
			return nil, err
		}
		results := []map[string]string{
			map[string]string{
				"identifier": identifier,
			},
		}

		return results, nil
	}
}

const (
	configBucket = "config"
	uuidKey      = "uuid"
)

// TODO: copied from extension.go
// create a common helper instead
func getIdentifierFromDB(db *bolt.DB) (string, error) {
	var identifier string
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(configBucket))
		uuidBytes := b.Get([]byte(uuidKey))
		gotID, err := uuid.ParseBytes(uuidBytes)

		// Use existing UUID
		if err == nil {
			identifier = gotID.String()
			return nil
		}

		// Generate new (random) UUID
		gotID, err = uuid.NewRandom()
		if err != nil {
			return errors.Wrap(err, "generating new UUID")
		}
		identifier = gotID.String()

		// Save new UUID
		err = b.Put([]byte(uuidKey), []byte(identifier))
		return errors.Wrap(err, "saving new UUID")
	})

	if err != nil {
		return "", err
	}

	return identifier, nil
}
