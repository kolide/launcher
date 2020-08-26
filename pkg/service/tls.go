package service

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

func makeTLSConfig(host string, insecureTLS bool, certPins [][]byte, rootPool *x509.CertPool, logger log.Logger) *tls.Config {
	conf := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureTLS,
		RootCAs:            rootPool,
		MinVersion:         tls.VersionTLS12,
	}

	if len(certPins) > 0 {
		conf.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			for _, chain := range verifiedChains {
				for _, cert := range chain {
					// Compare SHA256 hash of
					// SubjectPublicKeyInfo with each of
					// the pinned hashes.
					hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
					for _, pin := range certPins {
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
			level.Info(logger).Log(
				"msg", "no match found with pinned certificates",
				"err", "certificate pin validation failed",
			)
			return errors.New("no match found with pinned cert")
		}
	}

	return conf
}
