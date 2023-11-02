package service

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"

	"github.com/kolide/launcher/pkg/agent/types"
)

func makeTLSConfig(k types.Knapsack, rootPool *x509.CertPool) *tls.Config {
	conf := &tls.Config{
		ServerName:         k.KolideServerURL(),
		InsecureSkipVerify: k.InsecureTLS(),
		RootCAs:            rootPool,
		MinVersion:         tls.VersionTLS12,
	}

	if len(k.CertPins()) > 0 {
		conf.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			for _, chain := range verifiedChains {
				for _, cert := range chain {
					// Compare SHA256 hash of
					// SubjectPublicKeyInfo with each of
					// the pinned hashes.
					hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
					for _, pin := range k.CertPins() {
						if bytes.Equal(pin, hash[:]) {
							// Cert matches pin.
							return nil
						}
					}
				}
			}

			// Normally we wouldn't log and return an error, but
			// gRPC does not seem to expose the error in a way that
			// we can get at it later. At least this provides some
			// feedback to the user about what is going wrong.
			k.Slogger().Error("no match found with pinned certificates",
				"err", "certificate pin validation failed",
			)
			return errors.New("no match found with pinned cert")
		}
	}

	return conf
}
