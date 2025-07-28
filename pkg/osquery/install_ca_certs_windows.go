//go:build windows
// +build windows

package osquery

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// default CA certs for osquery. Copied from macOS's `/etc/ssl/cert.pem`
//
//go:embed ca-bundle.crt
var defaultCaCerts []byte

// InstallCaCerts returns the path to CA certificates.
// On Windows, it exports system certificates to a file for osquery to use.
func InstallCaCerts(directory string, slogger *slog.Logger) (string, error) {
	// Try to export Windows system certificates first
	systemCertsPath, err := exportSystemCaCerts(directory, slogger)
	if err == nil {
		return systemCertsPath, nil
	}

	// If exporting system certs fails, fall back to embedded bundle
	slogger.Log(context.TODO(), slog.LevelWarn,
		"Failed to export Windows system certificates, using embedded bundle instead",
		"err", err,
	)

	sum := sha256.Sum256(defaultCaCerts)
	caFile := filepath.Join(directory, fmt.Sprintf("ca-certs-embedded-%x.crt", sum))

	_, err = os.Stat(caFile)
	switch {
	case os.IsNotExist(err):
		return caFile, os.WriteFile(caFile, defaultCaCerts, 0444)
	case err != nil:
		return "", err
	}
	return caFile, nil
}

// exportSystemCaCerts exports Windows system CA certificates to a file
func exportSystemCaCerts(directory string, slog *slog.Logger) (string, error) {
	// Extract certificates directly from Windows Certificate Store
	certs, err := extractSystemCerts(slog)
	if err != nil {
		return "", fmt.Errorf("failed to extract system certificates: %w", err)
	}

	// Create PEM bundle from certificates
	var pemData []byte
	for _, cert := range certs {
		pemBlock := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		}
		pemData = append(pemData, pem.EncodeToMemory(pemBlock)...)
	}

	// Calculate hash of the exported certificates
	sum := sha256.Sum256(pemData)
	caFile := filepath.Join(directory, fmt.Sprintf("ca-certs-system-%x.crt", sum))

	// Check if file already exists with same content
	if _, err := os.Stat(caFile); err == nil {
		return caFile, nil
	}

	// Write the certificates to file
	if err := os.WriteFile(caFile, pemData, 0444); err != nil {
		return "", fmt.Errorf("failed to write system certificates: %w", err)
	}

	return caFile, nil
}
