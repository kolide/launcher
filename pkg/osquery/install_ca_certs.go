package osquery

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// defaultCaCerts is the fallback CA bundle for osquery, used when a host has no usable
// system trust store. It is Mozilla's root list as distributed by curl
// (https://curl.se/ca/cacert.pem).
//
//go:embed ca-bundle.crt
var defaultCaCerts []byte

// distrustedCaCertFingerprints lists SHA-256 fingerprints of CA certificates that must never
// be handed to osquery, even when the system trust store still contains them. osquery does
// not check revocation, so a compromised chain that remains in the system store is accepted
// unless we remove its root here. Note that we do not use the remote feature of osquery
// (https://osquery.readthedocs.io/en/latest/deployment/remote/); the usage of these certs for us
// is limited to e.g. the `curl` table.
//
// All three roots issued or cross-signed the eight eMudhra intermediates whose private keys
// were disclosed on 2026-10-17. See https://emudhra.we-have-a-problem.com/; fingerprints
// retrieved from https://emudhra.we-have-a-problem.com/roots.json.
var distrustedCaCertFingerprints = map[string]string{
	"86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b": "emSign ECC Root CA - G3",
	"7dd78d5f4f13459a83dff9abbba62edbaf6f2d102bf257fd712f4d9f2746ed8d": "emSign Root TLS CA - G3",
	"f2edaaff3d9b701fb4d14c9f285f8c0bad90fc3bcc1fa07d47f4718edaba34a5": "emSign Root CS CA - G3",
}

// InstallCaCerts returns the path to CA certificates for osquery. It prefers the system CA
// bundle over the embedded one so that customers can supply their own CAs. Either way, the
// bundle is written out with distrusted roots removed.
func InstallCaCerts(directory string, slogger *slog.Logger) (string, error) {
	systemCaCerts, err := readSystemCaCerts(slogger)
	if err == nil {
		return writeFilteredCaCerts(directory, "system", systemCaCerts)
	}

	slogger.Log(context.TODO(), slog.LevelWarn,
		"could not read system CA certificates, using embedded bundle instead",
		"err", err,
	)

	return writeFilteredCaCerts(directory, "embedded", defaultCaCerts)
}

// writeFilteredCaCerts writes pemBundle, minus any distrusted certificates, to a content-addressed
// file in directory and returns the path. source is included in the file name to distinguish
// system bundles from the embedded fallback.
func writeFilteredCaCerts(directory string, source string, pemBundle []byte) (string, error) {
	filtered := filterCaCerts(pemBundle)
	caFile := filepath.Join(directory, fmt.Sprintf("ca-certs-%s-%x.crt", source, sha256.Sum256(filtered)))

	if _, err := os.Stat(caFile); err == nil {
		return caFile, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking for existing CA certs file: %w", err)
	}

	// We write to a temp file and then rename it in place, in case this call is
	// racing with a call from another osquery instance. (This is currently
	// only an issue in tests.)
	tmpFile, err := os.CreateTemp(directory, "ca-certs-*.tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp CA certs file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // no-op once the rename succeeds

	if _, err := tmpFile.Write(filtered); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("writing temp CA certs file: %w", err)
	}
	if err := tmpFile.Chmod(0444); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("setting permissions on temp CA certs file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("closing temp CA certs file: %w", err)
	}

	if err := os.Rename(tmpPath, caFile); err != nil {
		// A concurrent caller may have already published an identical file -- the name is
		// derived from the content, so if it exists now, it is the file we wanted.
		if _, statErr := os.Stat(caFile); statErr == nil {
			return caFile, nil
		}
		return "", fmt.Errorf("renaming temp CA certs file into place: %w", err)
	}

	return caFile, nil
}

// filterCaCerts returns pemBundle with any distrusted certificates removed. Everything else,
// including non-certificate PEM blocks and any text between blocks, is passed through as-is.
func filterCaCerts(pemBundle []byte) []byte {
	out := make([]byte, 0, len(pemBundle))
	rest := pemBundle

	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			return append(out, rest...)
		}

		// pem.Decode skips any text preceding the block; consumed covers both that text and the block.
		consumed := rest[:len(rest)-len(remaining)]
		if block.Type != "CERTIFICATE" || !isDistrustedCaCert(block.Bytes) {
			out = append(out, consumed...)
		}

		rest = remaining
	}
}

func isDistrustedCaCert(der []byte) bool {
	sum := sha256.Sum256(der)
	_, distrusted := distrustedCaCertFingerprints[hex.EncodeToString(sum[:])]
	return distrusted
}
