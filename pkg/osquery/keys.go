package osquery

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// This file is temporary, until we bring in a new library for v0.13

func rsaRandomKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

func rsaFingerprint(keyRaw interface{}) (string, error) {
	var pub *rsa.PublicKey

	switch key := keyRaw.(type) {
	case *rsa.PrivateKey:
		pub = key.Public().(*rsa.PublicKey)
	case *rsa.PublicKey:
		pub = key
	default:
		return "", errors.New("cannot fingerprint that type")
	}

	pkix, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshalling to PKIX: %w", err)
	}

	sum := sha256.Sum256(pkix)

	out := ""
	for i := 0; i < 32; i++ {
		if i > 0 {
			out += ":"
		}
		out += fmt.Sprintf("%02x", sum[i])
	}

	return out, nil
}

func RsaPublicKeyToPem(key *rsa.PrivateKey, out io.Writer) error {
	pubASN1, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return fmt.Errorf("pkix marshalling: %w", err)
	}

	return pem.Encode(out, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})
}
