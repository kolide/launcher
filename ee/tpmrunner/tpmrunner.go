//go:build !darwin
// +build !darwin

package tpmrunner

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kolide/krypto/pkg/tpm"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/pkg/backoff"
)

type (
	tpmRunner struct {
		signer        crypto.Signer
		mux           sync.Mutex
		signerCreator tpmSignerCreator
		store         types.GetterSetterDeleter
		slogger       *slog.Logger
		interrupt     chan struct{}
		interrupted   atomic.Bool
		machineHasTpm atomic.Bool
	}

	// tpmSignerCreator is an interface for creating and loading TPM signers
	// useful for mocking in tests
	tpmSignerCreator interface {
		CreateKey(opts ...tpm.TpmSignerOption) (private []byte, public []byte, err error)
		New(private, public []byte) (crypto.Signer, error)
	}

	// defaultTpmSignerCreator is the default implementation of tpmSignerCreator
	// using the tpm package
	defaultTpmSignerCreator struct{}

	// tpmRunnerOption is a functional option for tpmRunner
	// useful for setting dependencies in tests
	tpmRunnerOption func(*tpmRunner)
)

// CreateKey creates a new TPM key
func (d defaultTpmSignerCreator) CreateKey(opts ...tpm.TpmSignerOption) (private []byte, public []byte, err error) {
	return tpm.CreateKey()
}

// New creates a new TPM signer
func (d defaultTpmSignerCreator) New(private, public []byte) (crypto.Signer, error) {
	return tpm.New(private, public)
}

func New(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, opts ...tpmRunnerOption) (*tpmRunner, error) {
	tpmRunner := &tpmRunner{
		store:         store,
		slogger:       slogger.With("component", "tpmrunner"),
		interrupt:     make(chan struct{}),
		signerCreator: defaultTpmSignerCreator{},
	}

	// assume we have a tpm until we know otherwise
	hasTPM := true

	// on linux the TPM is at /dev/tpm0
	// if it doesn't exist, we don't have a TPM
	if runtime.GOOS == "linux" {
		_, err := os.Stat("/dev/tpm0")
		hasTPM = err == nil

		if !hasTPM {
			slogger.Log(ctx, slog.LevelInfo,
				"no tpm found",
				"err", err,
			)
		}
	}

	tpmRunner.machineHasTpm.Store(hasTPM)

	for _, opt := range opts {
		opt(tpmRunner)
	}

	return tpmRunner, nil
}

// Public returns the public key of the current console user
// creating and peristing a new one if needed
func (tr *tpmRunner) Execute() error {
	durationCounter := backoff.NewMultiplicativeDurationCounter(time.Second, time.Minute)
	retryTicker := time.NewTicker(durationCounter.Next())
	defer retryTicker.Stop()

	for {
		// try to create signer if we don't have one
		if tr.signer == nil && tr.machineHasTpm.Load() {
			ctx := context.Background()
			if err := tr.loadOrCreateKeys(ctx); err != nil {
				tr.slogger.Log(ctx, slog.LevelInfo,
					"loading or creating keys in execute loop",
					"err", err,
				)
			}
		}

		if tr.signer != nil || !tr.machineHasTpm.Load() {
			retryTicker.Stop()
		}

		select {
		case <-retryTicker.C:
			retryTicker.Reset(durationCounter.Next())
			continue
		case <-tr.interrupt:
			tr.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupt received, exiting tpm signer execute loop",
			)
			return nil
		}
	}
}

func (tr *tpmRunner) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if tr.interrupted.Load() {
		return
	}

	tr.interrupted.Store(true)

	// Tell the execute loop to stop checking, and exit
	tr.interrupt <- struct{}{}
}

// Public returns the public hardware key
func (tr *tpmRunner) Public() crypto.PublicKey {
	if !tr.machineHasTpm.Load() {
		return nil
	}

	if tr.signer != nil {
		return tr.signer.Public()
	}

	if err := tr.loadOrCreateKeys(context.Background()); err != nil {
		tr.slogger.Log(context.Background(), slog.LevelInfo,
			"loading or creating keys in public call",
			"err", err,
		)

		return nil
	}

	return tr.signer.Public()
}

func (tr *tpmRunner) Type() string {
	return "tpm"
}

func (tr *tpmRunner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if tr.signer == nil {
		return nil, errors.New("no signer available")
	}

	return tr.signer.Sign(rand, digest, opts)
}

// This duplicates some of pkg/osquery/extension.go but that feels like the wrong place.
// Really, we should have a simpler interface over a storage layer.
const (
	privateEccData = "privateEccData"
	publicEccData  = "publicEccData"
)

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
func clearKeyData(slogger *slog.Logger, deleter types.Deleter) {
	slogger.Log(context.TODO(), slog.LevelInfo,
		"clearing keys",
	)
	_ = deleter.Delete([]byte(privateEccData), []byte(publicEccData))
}

func (tr *tpmRunner) loadOrCreateKeys(ctx context.Context) error {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	tr.mux.Lock()
	defer tr.mux.Unlock()

	// tpm signer was already created
	if tr.signer != nil {
		return nil
	}

	priData, pubData, err := fetchKeyData(tr.store)
	if err != nil {
		thisErr := fmt.Errorf("fetching key data for data store: %w", err)
		observability.SetError(span, thisErr)
		return thisErr
	}

	if pubData == nil || priData == nil {
		var err error
		priData, pubData, err = tr.signerCreator.CreateKey()
		if err != nil {

			if isTerminalTPMError(err) {
				tr.machineHasTpm.Store(false)

				tr.slogger.Log(ctx, slog.LevelInfo,
					"terminal tpm error, not retrying",
					"err", err,
				)

				span.AddEvent("tpm_not_found")
				return err
			}

			thisErr := fmt.Errorf("creating key: %w", err)
			observability.SetError(span, thisErr)

			clearKeyData(tr.slogger, tr.store)
			return thisErr
		}

		if err := storeKeyData(tr.store, priData, pubData); err != nil {
			thisErr := fmt.Errorf("storing key data: %w", err)
			observability.SetError(span, thisErr)

			clearKeyData(tr.slogger, tr.store)
			return thisErr
		}

		tr.slogger.Log(ctx, slog.LevelInfo,
			"new tpm keys generated",
		)
		span.AddEvent("generated_new_tpm_keys")
	}

	k, err := tr.signerCreator.New(priData, pubData)
	if err != nil {
		thisErr := fmt.Errorf("creating tpm signer: %w", err)
		observability.SetError(span, thisErr)
		return thisErr
	}

	tr.signer = k

	tr.slogger.Log(ctx, slog.LevelDebug,
		"tpm signer created",
	)
	span.AddEvent("created_tpm_signer")

	return nil
}
