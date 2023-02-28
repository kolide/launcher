package agent

import (
	"crypto"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/keys"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
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

func SetupKeys(logger log.Logger, store types.GetterSetterDeleter) error {
	logger = log.With(logger, "component", "agentkeys")

	var err error

	// Always setup a local key
	localDbKeys, err = keys.SetupLocalDbKey(logger, store)
	if err != nil {
		return fmt.Errorf("setting up local db keys: %w", err)
	}

	err = backoff.WaitFor(func() error {
		hwKeys, err := setupHardwareKeys(logger, store)
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
	privateEccData = "privateEccData"
	publicEccData  = "publicEccData"
)

func fetchKeyData(getter types.Getter) ([]byte, []byte, error) {
	pri, err := getter.Get([]byte(privateEccData))
	if err != nil {
		return nil, nil, err
	}

	pub, err := getter.Get([]byte(publicEccData))
	if err != nil {
		return nil, nil, err
	}

	return pri, pub, nil
}

func storeKeyData(setter types.Setter, pri, pub []byte) error {
	if pri != nil {
		if err := setter.Set([]byte(privateEccData), pri); err != nil {
			return err
		}
	}

	if pub != nil {
		if err := setter.Set([]byte(publicEccData), pub); err != nil {
			return err
		}
	}

	return nil
}

// clearKeyData is used to clear the keys as part of error handling around new keys. It is not intended to be called
// regularly, and since the path that calls it is around DB errors, it has no error handling.
func clearKeyData(logger log.Logger, deleter types.Deleter) {
	level.Info(logger).Log("msg", "Clearing keys")
	_ = deleter.Delete([]byte(privateEccData))
	_ = deleter.Delete([]byte(publicEccData))
}
