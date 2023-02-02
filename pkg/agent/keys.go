package agent

import (
	"crypto"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/keys"
	"github.com/kolide/launcher/pkg/backoff"
	"go.etcd.io/bbolt"
)

type keyInt interface {
	crypto.Signer
	Type() string
}

var hardwareKeys keyInt = keys.Noop
var localDbKeys keyInt = keys.Noop

func HardwareKeys() keyInt {
	return hardwareKeys
}

func LocalDbKeys() keyInt {
	return localDbKeys
}

func SetupKeys(logger log.Logger, db *bbolt.DB) error {
	logger = log.With(logger, "component", "agentkeys")

	var err error

	// Always setup a local key
	localDbKeys, err = keys.SetupLocalDbKey(logger, db)
	if err != nil {
		return fmt.Errorf("setting up local db keys: %w", err)
	}

	err = backoff.WaitFor(func() error {
		hwKeys, err := setupHardwareKeys(logger, db)
		if err != nil {
			return err
		}
		hardwareKeys = hwKeys
		return nil
	}, 1*time.Second, 250*time.Millisecond)

	if err != nil {
		// Use of hardware keys is not fully implemented as of 2023-02-01, so log an error and move on
		level.Info(logger).Log("msg", "failed to setting up hardware keys", "err", err)
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

		// It's not really clear what we should do if this is called with a nil pri or pub. We
		// could delete the key data, but that feels like the wrong thing -- what if there's a
		// weird caller error? So, in the event of nils, we skip the write. We may revisit this
		// as we learn more

		if pri != nil {
			if err := b.Put([]byte(privateEccData), pri); err != nil {
				return err
			}
		}

		if pub != nil {
			if err := b.Put([]byte(publicEccData), pub); err != nil {
				return err
			}
		}

		return nil
	})
}

// clearKeyData is used to clear the keys as part of error handling around new keys. It is not intended to be called
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
