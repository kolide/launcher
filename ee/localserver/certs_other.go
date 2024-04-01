//go:build !darwin
// +build !darwin

package localserver

import (
	"context"
	"crypto/x509"
	"errors"
)

func addCertToKeyStore(ctx context.Context, certRaw []byte, cert *x509.Certificate) error {
	return errors.New("not yet implemented")
}
