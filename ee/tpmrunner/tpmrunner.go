package tpmrunner

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kolide/krypto/pkg/tpm"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

type tpmRunner struct {
	signer        crypto.Signer
	signerCreator tpmSignerCreator
	store         types.GetterSetterDeleter
	slogger       *slog.Logger
	interrupt     chan struct{}
	interrupted   bool
}

type tpmSignerCreator interface {
	CreateKey(opts ...tpm.TpmSignerOption) (private []byte, public []byte, err error)
	New(private, public []byte) (crypto.Signer, error)
}

type defaultTpmSignerCreator struct{}

func (d defaultTpmSignerCreator) CreateKey(opts ...tpm.TpmSignerOption) (private []byte, public []byte, err error) {
	return tpm.CreateKey()
}

func (d defaultTpmSignerCreator) New(private, public []byte) (crypto.Signer, error) {
	return tpm.New(private, public)
}

type tpmRunnerOption func(*tpmRunner)

func New(ctx context.Context, slogger *slog.Logger, store types.GetterSetterDeleter, opts ...tpmRunnerOption) (*tpmRunner, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	_, _, err := fetchKeyData(store)
	if err != nil {
		return nil, err
	}

	tpmRunner := &tpmRunner{
		store:         store,
		slogger:       slogger.With("component", "tpmrunner"),
		interrupt:     make(chan struct{}),
		signerCreator: defaultTpmSignerCreator{},
	}

	for _, opt := range opts {
		opt(tpmRunner)
	}

	return tpmRunner, nil
}

// Public returns the public key of the current console user
// creating and peristing a new one if needed
func (tr *tpmRunner) Execute() error {
	currentRetryInterval, maxRetryInterval := 1*time.Second, 1*time.Minute
	retryTicker := time.NewTicker(currentRetryInterval)
	defer retryTicker.Stop()

	for {

		signer, err := tr.fetchCreateKeys(context.TODO())
		if err != nil {
			tr.slogger.Log(context.TODO(), slog.LevelError,
				"creating tpm signer, will retry",
				"err", err,
			)

			if currentRetryInterval < maxRetryInterval {
				currentRetryInterval += time.Second
			}
		} else {
			tr.signer = signer
			retryTicker.Stop()
		}

		select {
		case <-retryTicker.C:
			continue
		case <-tr.interrupt:
			tr.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupt received, exiting secure enclave signer execute loop",
			)
			return nil
		}
	}
}

func (tr *tpmRunner) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if tr.interrupted {
		return
	}

	tr.interrupted = true

	// Tell the execute loop to stop checking, and exit
	tr.interrupt <- struct{}{}
}

// Public returns the public key of the current console user
// creating and peristing a new one if needed
func (tr *tpmRunner) Public() crypto.PublicKey {
	if tr.signer == nil {
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

func (tr *tpmRunner) fetchCreateKeys(ctx context.Context) (crypto.Signer, error) {
	priData, pubData, err := fetchKeyData(tr.store)
	if err != nil {
		return nil, fmt.Errorf("unabled to access key data store: %w", err)
	}

	if pubData == nil || priData == nil {
		tr.slogger.Log(ctx, slog.LevelInfo,
			"generating new keys",
		)

		var err error
		priData, pubData, err = tr.signerCreator.CreateKey()
		if err != nil {
			clearKeyData(tr.slogger, tr.store)
			return nil, fmt.Errorf("creating key: %w", err)
		}

		if err := storeKeyData(tr.store, priData, pubData); err != nil {
			clearKeyData(tr.slogger, tr.store)
			return nil, fmt.Errorf("storing key: %w", err)
		}
	}

	k, err := tr.signerCreator.New(priData, pubData)
	if err != nil {
		return nil, fmt.Errorf("creating tpm signer: %w", err)
	}

	return k, nil
}
