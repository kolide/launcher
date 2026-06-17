package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"

	"github.com/kolide/launcher/v2/ee/agent/types"
)

func makeTLSConfig(k types.Knapsack, rootPool *x509.CertPool) *tls.Config {

	hostname := k.KolideServerURL()

	// ServerName must be host-only: certificate SANs (DNS or IP) never encode a
	// port, and Go matches ServerName against the SANs literally. SplitHostPort
	// correctly handles bracketed IPv6 literals; it errors when no port is
	// present, in which case we leave hostname untouched.
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}

	conf := &tls.Config{
		ServerName:         hostname,
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
			k.Slogger().Log(context.TODO(), slog.LevelError,
				"no match found with pinned certificates",
				"err", "certificate pin validation failed",
			)
			return errors.New("no match found with pinned cert")
		}
	}

	return conf
}
