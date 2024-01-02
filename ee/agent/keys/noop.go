package keys

import (
	"crypto"
	"errors"
	"io"
)

// noopKeys is a no-op implementation of keyInt. It's here to be a default
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

var Noop = noopKeys{}
