package agent

import (
	"crypto"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/keys"
	"go.etcd.io/bbolt"
)

type keyInt interface {
	crypto.Signer
	//Type() string // Not Yet Supported by Krypto
}

var Keys keyInt = keys.Noop
var LocalDbKeys keyInt = keys.Noop

func SetupKeys(logger log.Logger, db *bbolt.DB) error {
	var err error

	// Always setup a local key
	LocalDbKeys, err = keys.SetupLocalDbKey(logger, db)
	if err != nil {
		return fmt.Errorf("setting up local db keys: %w", err)
	}

	Keys, err = setupHardwareKeys(logger, db)
	if err != nil {
		// Now this is a conundrum. What should we do if there's a hardware keying error?
		// We could return the error, and abort, but that would block launcher for working in places
		// without keys. Inatead, we log the error and set Keys to the localDb key.
		level.Info(logger).Log("msg", "setting up hardware keys", "err", err)
		Keys = LocalDbKeys
	}

	return nil
}

// This duplicates some of pkg/osquery/extension.go but that feels like the wrong place.
// Really, we should have a simpler interface over a storage layer.
const (
	bucketName     = "config"
	privateEccData = "privateEccData"
	publicEccData  = "publicEccData"
)

func fetchKeyData(db *bbolt.DB) ([]byte, []byte, error) {
	var pri []byte
	var pub []byte

	if err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}

		pri = b.Get([]byte(privateEccData))
		pub = b.Get([]byte(publicEccData))

		return nil
	}); err != nil {
		return nil, nil, err
	}

	return pri, pub, nil
}

func storeKeyData(db *bbolt.DB, pri, pub []byte) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}

		if err := b.Put([]byte(privateEccData), pri); err != nil {
			return err
		}

		if err := b.Put([]byte(publicEccData), pub); err != nil {
			return err
		}

		return nil
	})
}

// clearKeyData is used to clear the keys as part of error handling around new keys. It is not intented to be called
// regularly, and since the path that calls it is around DB errors, it has no error handling.
func clearKeyData(logger log.Logger, db *bbolt.DB) {
	level.Info(logger).Log("msg", "Clearing keys")
	_ = db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}

		_ = b.Delete([]byte(privateEccData))
		_ = b.Delete([]byte(publicEccData))

		return nil
	})
}
