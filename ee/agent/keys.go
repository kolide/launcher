package agent

import (
	"context"
	"crypto"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/agent/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/traces"
)

type keyInt interface {
	crypto.Signer
	Type() string
}

type KeyIntHardware interface {
	keyInt
	SignConsoleUser(ctx context.Context, challenge, data, serverPubkey []byte, baseNonce string) ([]byte, error)
}

var hardwareKeys KeyIntHardware = keys.Noop
var localDbKeys keyInt = keys.Noop

func HardwareKeys() KeyIntHardware {
	return hardwareKeys
}

func LocalDbKeys() keyInt {
	return localDbKeys
}

func SetupKeys(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, skipHardwareKeys bool) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	slogger = slogger.With("component", "agentkeys")

	var err error

	// Always setup a local key
	localDbKeys, err = keys.SetupLocalDbKey(slogger, store)
	if err != nil {
		return fmt.Errorf("setting up local db keys: %w", err)
	}

	if skipHardwareKeys {
		return nil
	}

	err = backoff.WaitFor(func() error {
		hwKeys, err := setupHardwareKeys(ctx, slogger, store)
		if err != nil {
			return err
		}
		hardwareKeys = hwKeys
		return nil
	}, 1*time.Second, 250*time.Millisecond)

	if err != nil {
		// Use of hardware keys is not fully implemented as of 2023-02-01, so log an error and move on
		slogger.Log(context.TODO(), slog.LevelInfo,
			"failed setting up hardware keys",
			"err", err,
		)
	}

	return nil
}

// This duplicates some of pkg/osquery/extension.go but that feels like the wrong place.
// Really, we should have a simpler interface over a storage layer.
const (
	privateEccData = "privateEccData" // nolint:unused
	publicEccData  = "publicEccData"  // nolint:unused
)

// nolint:unused
func fetchKeyData(store types.Getter) ([]byte, []byte, error) {
	pri, err := store.Get([]byte(privateEccData))
	if err != nil {
		return nil, nil, err
	}

	pub, err := store.Get([]byte(publicEccData))
	if err != nil {
		return nil, nil, err
	}

	return pri, pub, nil
}

// nolint:unused
func storeKeyData(store types.Setter, pri, pub []byte) error {
	if pri != nil {
		if err := store.Set([]byte(privateEccData), pri); err != nil {
			return err
		}
	}

	if pub != nil {
		if err := store.Set([]byte(publicEccData), pub); err != nil {
			return err
		}
	}

	return nil
}

// clearKeyData is used to clear the keys as part of error handling around new keys. It is not intended to be called
// regularly, and since the path that calls it is around DB errors, it has no error handling.
// nolint:unused
func clearKeyData(slogger *slog.Logger, deleter types.Deleter) {
	slogger.Log(context.TODO(), slog.LevelInfo,
		"clearing keys",
	)
	_ = deleter.Delete([]byte(privateEccData), []byte(publicEccData))
}
