//go:build darwin
// +build darwin

// Package secureenclaverunner is a runner that manages the secure enclave key for the current console user.
// It's a runner because we can not perform any secure enclave operations without a console user logged in, so we keep trying until we have one.
// In order to use the secure enclave, you need a macOS app that is signed with the correct entitlements.
// There are some instructions for signing with a developer key here: https://github.com/kolide/krypto/blob/main/pkg/secureenclave/test_app_resources/readme.md.
// It's important to note that secure enclave keys can only be accessed by an app with the signature that created the key, so if you attempt to access keys created by
// the production app with your development signature, you will get an error.
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

type keyEntry struct {
	pubKey                  *ecdsa.PublicKey
	verifiedInSecureEnclave bool
}

type secureEnclaveRunner struct {
	uidPubKeyMap        map[string]*keyEntry
	uidPubKeyMapMux     *sync.Mutex
	secureEnclaveClient secureEnclaveClient
	store               types.GetterSetterDeleter
	slogger             *slog.Logger
	interrupt           chan struct{}
	interrupted         atomic.Bool
	noConsoleUsersDelay time.Duration
}

type secureEnclaveClient interface {
	CreateSecureEnclaveKey(ctx context.Context, uid string) (*ecdsa.PublicKey, error)
	VerifySecureEnclaveKey(ctx context.Context, uid string, pubKey *ecdsa.PublicKey) (bool, error)
}

func New(_ context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, secureEnclaveClient secureEnclaveClient) (*secureEnclaveRunner, error) {
	return &secureEnclaveRunner{
		uidPubKeyMap:        make(map[string]*keyEntry),
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

	entry, ok := ser.uidPubKeyMap[cu.Uid]

	// found key, already verified in secure enclave
	if ok && entry.verifiedInSecureEnclave {
		ser.slogger.Log(ctx, slog.LevelDebug,
			"found existing key for console user",
			"uid", cu.Uid,
			"verified_in_secure_enclave", entry.verifiedInSecureEnclave,
		)
		span.AddEvent("found_existing_verified_key_for_console_user")
		return entry.pubKey, nil
	}

	// found key, but not verified in secure enclave
	if ok {
		verfied, err := ser.secureEnclaveClient.VerifySecureEnclaveKey(ctx, cu.Uid, entry.pubKey)

		// got err, cannot determine if key is valid
		if err != nil {
			traces.SetError(span, fmt.Errorf("verifying existing key: %w", err))
			span.AddEvent("verifying_existing_key_for_console_user_failed")
			return nil, fmt.Errorf("verifying existing key: %w", err)
		}

		// key exists in secure enclave
		if verfied {
			ser.slogger.Log(ctx, slog.LevelDebug,
				"verified key exists in secure enclave",
				"uid", cu.Uid,
			)
			entry.verifiedInSecureEnclave = verfied
			return entry.pubKey, nil
		}

		// key does not exist in secure enclave
		span.AddEvent("key_does_not_exist_in_secure_enclave")
		delete(ser.uidPubKeyMap, cu.Uid)
		ser.slogger.Log(ctx, slog.LevelInfo,
			"key does not exist in secure enclave, deleting from store",
			"uid", cu.Uid,
		)

		if err := ser.save(); err != nil {
			traces.SetError(span, fmt.Errorf("saving secure enclave signer: %w", err))
			ser.slogger.Log(ctx, slog.LevelError,
				"error saving secure enclave signer after key deletion",
				"err", err,
			)

			return nil, fmt.Errorf("saving secure enclave signer: %w", err)
		}

		span.AddEvent("deleted_key_for_console_user")
	}

	key, err := ser.secureEnclaveClient.CreateSecureEnclaveKey(ctx, cu.Uid)
	if err != nil {
		traces.SetError(span, fmt.Errorf("creating key: %w", err))
		return nil, fmt.Errorf("creating key: %w", err)
	}

	ser.slogger.Log(ctx, slog.LevelInfo,
		"created new key for console user",
		"uid", cu.Uid,
	)
	span.AddEvent("created_new_key_for_console_user")

	ser.uidPubKeyMap[cu.Uid] = &keyEntry{
		pubKey: key,
		// since we just created, we can verify that it's in the secure enclave
		verifiedInSecureEnclave: true,
	}

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

	for uid, entry := range ser.uidPubKeyMap {
		// It's important to note that when we are marshalling the key, we are not saving whether
		// or not it was verified in the secure enclave. We want this to happen on every launcher run
		// so that if the db was copied to a new machine or the secure enclave was reset,
		// we don't falsely assume the key is valid
		pubKeyBytes, err := x509.MarshalPKIXPublicKey(entry.pubKey)
		if err != nil {
			return nil, fmt.Errorf("marshalling to PXIX public key: %w", err)
		}

		keyMap[uid] = base64.StdEncoding.EncodeToString(pubKeyBytes)
	}

	return json.Marshal(keyMap)
}

func (ser *secureEnclaveRunner) UnmarshalJSON(data []byte) error {
	if ser.uidPubKeyMap == nil {
		ser.uidPubKeyMap = make(map[string]*keyEntry)
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

		ser.uidPubKeyMap[k] = &keyEntry{
			pubKey: ecdsaPubKey,
			// we can't verify the key here because we can't be sure a user is
			// logged in and the secure enclave is available
			verifiedInSecureEnclave: false,
		}
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
