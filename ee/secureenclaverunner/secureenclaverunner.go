//go:build darwin
// +build darwin

package secureenclaverunner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/user"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/consoleuser"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/traces"
)

const (
	publicEccDataKey = "publicEccData"
)

type secureEnclaveRunner struct {
	uidPubKeyMap        map[string]*ecdsa.PublicKey
	uidPubKeyMapMux     *sync.Mutex
	secureEnclaveClient secureEnclaveClient
	store               types.GetterSetterDeleter
	slogger             *slog.Logger
	interrupt           chan struct{}
	interrupted         atomic.Bool
	noConsoleUsersDelay time.Duration
}

type secureEnclaveClient interface {
	CreateSecureEnclaveKey(uid string) (*ecdsa.PublicKey, error)
}

func New(_ context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, secureEnclaveClient secureEnclaveClient) (*secureEnclaveRunner, error) {
	return &secureEnclaveRunner{
		uidPubKeyMap:        make(map[string]*ecdsa.PublicKey),
		store:               store,
		secureEnclaveClient: secureEnclaveClient,
		slogger:             slogger.With("component", "secureenclaverunner"),
		uidPubKeyMapMux:     &sync.Mutex{},
		interrupt:           make(chan struct{}),
		noConsoleUsersDelay: 15 * time.Second,
	}, nil
}

// Public returns the public key of the current console user
// creating and peristing a new one if needed
func (ser *secureEnclaveRunner) Execute() error {
	data, err := ser.store.Get([]byte(publicEccDataKey))
	if err != nil {
		return fmt.Errorf("getting public ecc data from store: %w", err)
	}

	if data != nil {
		if err := json.Unmarshal(data, ser); err != nil {
			ser.slogger.Log(context.TODO(), slog.LevelError,
				"unable to unmarshal secure enclave signer, data may be corrupt, wiping",
				"err", err,
			)

			if err := ser.store.Delete([]byte(publicEccDataKey)); err != nil {
				ser.slogger.Log(context.TODO(), slog.LevelError,
					"unable to unmarshal secure enclave signer, data may be corrupt, wiping",
					"err", err,
				)
			}
		}
	}

	durationCounter := backoff.NewMultiplicativeDurationCounter(time.Second, time.Minute)
	retryTicker := time.NewTicker(time.Second)
	defer retryTicker.Stop()

	inNoConsoleUsersState := false

	for {
		ctx := context.Background()
		_, err := ser.currentConsoleUserKey(ctx)

		switch {

		// don't have console user, so wait longer to retry
		case errors.Is(err, noConsoleUsersError{}):
			inNoConsoleUsersState = true
			retryTicker.Reset(ser.noConsoleUsersDelay)

		// now that we have a console user, restart the backoff
		case err != nil && inNoConsoleUsersState:
			durationCounter.Reset()
			retryTicker.Reset(durationCounter.Next())
			inNoConsoleUsersState = false

		// we have console user, but failed to get key
		case err != nil:
			retryTicker.Reset(durationCounter.Next())

		// success
		default:
			retryTicker.Stop()
		}

		// log any errors
		if err != nil {
			ser.slogger.Log(ctx, slog.LevelDebug,
				"getting current console user key",
				"err", err,
			)
		}

		select {
		case <-retryTicker.C:
			continue
		case <-ser.interrupt:
			ser.slogger.Log(ctx, slog.LevelDebug,
				"interrupt received, exiting secure enclave signer execute loop",
			)
			return nil
		}
	}
}

func (ser *secureEnclaveRunner) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if ser.interrupted.Load() {
		return
	}

	ser.interrupted.Store(true)

	// Tell the execute loop to stop checking, and exit
	ser.interrupt <- struct{}{}
}

// Public returns the public key of the current console user
// creating and peristing a new one if needed
func (ser *secureEnclaveRunner) Public() crypto.PublicKey {
	k, err := ser.currentConsoleUserKey(context.TODO())
	if err != nil {
		ser.slogger.Log(context.TODO(), slog.LevelError,
			"getting public key",
			"err", err,
		)
		return nil
	}

	// currentConsoleUserKey may return no error and a nil pointer where the inability
	// to get the key is expected (see logic around calling firstConsoleUser). In this case,
	// k will be a "typed" nil, as an uninitialized pointer to a ecdsa.PublicKey. We're returning
	// this typed nil assigned as the crypto.PublicKey interface. This means that the interface's value
	// will be nil, but it's underlying type will not be - so it will pass nil checks but panic
	// when typecasted later. Explicitly return an untyped nil in this case to prevent confusion and panics later
	if k == nil {
		return nil
	}

	return k
}

func (ser *secureEnclaveRunner) Type() string {
	return "secure_enclave"
}

func (ser *secureEnclaveRunner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (ser *secureEnclaveRunner) currentConsoleUserKey(ctx context.Context) (*ecdsa.PublicKey, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ser.uidPubKeyMapMux.Lock()
	defer ser.uidPubKeyMapMux.Unlock()

	cu, err := firstConsoleUser(ctx)
	if err != nil {
		ser.slogger.Log(ctx, slog.LevelDebug,
			"getting first console user, expected when root launcher running without a logged in console user",
			"err", err,
		)

		span.AddEvent("no_console_user_found")
		return nil, nil
	}

	key, ok := ser.uidPubKeyMap[cu.Uid]
	if ok {
		ser.slogger.Log(ctx, slog.LevelDebug,
			"found existing key for console user",
			"uid", cu.Uid,
		)
		span.AddEvent("found_existing_key_for_console_user")
		return key, nil
	}

	key, err = ser.secureEnclaveClient.CreateSecureEnclaveKey(cu.Uid)
	if err != nil {
		traces.SetError(span, fmt.Errorf("creating key: %w", err))
		return nil, fmt.Errorf("creating key: %w", err)
	}

	ser.slogger.Log(ctx, slog.LevelInfo,
		"created new key for console user",
		"uid", cu.Uid,
	)
	span.AddEvent("created_new_key_for_console_user")

	ser.uidPubKeyMap[cu.Uid] = key
	if err := ser.save(); err != nil {
		delete(ser.uidPubKeyMap, cu.Uid)
		traces.SetError(span, fmt.Errorf("saving secure enclave signer: %w", err))
		return nil, fmt.Errorf("saving secure enclave signer: %w", err)
	}

	span.AddEvent("saved_key_for_console_user")
	return key, nil
}

func (ser *secureEnclaveRunner) MarshalJSON() ([]byte, error) {
	keyMap := make(map[string]string)

	for uid, pubKey := range ser.uidPubKeyMap {
		pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
		if err != nil {
			return nil, fmt.Errorf("marshalling to PXIX public key: %w", err)
		}

		keyMap[uid] = base64.StdEncoding.EncodeToString(pubKeyBytes)
	}

	return json.Marshal(keyMap)
}

func (ser *secureEnclaveRunner) UnmarshalJSON(data []byte) error {
	if ser.uidPubKeyMap == nil {
		ser.uidPubKeyMap = make(map[string]*ecdsa.PublicKey)
	}

	var keyMap map[string]string
	if err := json.Unmarshal(data, &keyMap); err != nil {
		return fmt.Errorf("unmarshalling key data: %w", err)
	}

	for k, v := range keyMap {
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return fmt.Errorf("decoding base64: %w", err)
		}

		pubKey, err := x509.ParsePKIXPublicKey(decoded)
		if err != nil {
			return fmt.Errorf("parsing PXIX public key: %w", err)
		}

		ecdsaPubKey, ok := pubKey.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("public key is not ecdsa")
		}

		ser.uidPubKeyMap[k] = ecdsaPubKey
	}

	return nil
}

type noConsoleUsersError struct{}

func (noConsoleUsersError) Error() string {
	return "no console users found"
}

func firstConsoleUser(ctx context.Context) (*user.User, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	c, err := consoleuser.CurrentUsers(ctx)
	if err != nil {
		traces.SetError(span, fmt.Errorf("getting current users: %w", err))
		return nil, fmt.Errorf("getting current users: %w", err)
	}

	if len(c) == 0 {
		traces.SetError(span, errors.New("no console users found"))
		return nil, noConsoleUsersError{}
	}

	return c[0], nil
}

func (ser *secureEnclaveRunner) save() error {
	json, err := json.Marshal(ser)
	if err != nil {
		return fmt.Errorf("marshaling secure enclave signer: %w", err)
	}

	if err := ser.store.Set([]byte(publicEccDataKey), json); err != nil {
		return fmt.Errorf("setting public ecc data: %w", err)
	}

	return nil
}
