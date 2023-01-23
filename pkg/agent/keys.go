package agent

import (
	"crypto"
	"errors"
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"go.etcd.io/bbolt"
)

type keyInt interface {
	crypto.Signer
	//Type() string // Not Yet Supported by Krypto
}

type noopKeys struct {
}

func (n noopKeys) Sign(_ io.Reader, _ []byte, _ crypto.SignerOpts) (signature []byte, err error) {
	return nil, errors.New("Can't sign. Unconfigured keys")
}

func (n noopKeys) Public() crypto.PublicKey {
	return nil
}

func (n noopKeys) Type() string {
	return "noop"
}

var Keys keyInt = noopKeys{}

func SetupKeys(logger log.Logger, db *bbolt.DB) error {
	// FIXME: How do we detect failure is _hardware_ and fallback to local keys?
	return setupHardwareKeys(logger, db)
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
