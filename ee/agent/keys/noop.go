package keys

import (
	"context"
	"crypto"
	"errors"
	"io"
)

// noopKeys is a no-op implementation of keyInt. It's here to be a default
type noopKeys struct {
}

func (n noopKeys) Sign(_ io.Reader, _ []byte, _ crypto.SignerOpts) (signature []byte, err error) {
	return nil, errors.New("can't sign, unconfigured keys")
}

func (n noopKeys) Public() crypto.PublicKey {
	return nil
}

func (n noopKeys) Type() string {
	return "noop"
}

func (n noopKeys) SignConsoleUser(_ context.Context, _, _, _ []byte, _ string) ([]byte, error) {
	return nil, errors.New("can't sign with console user, unconfigured keys")
}

var Noop = noopKeys{}
