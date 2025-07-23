//go:build windows
// +build windows

package osquery

import (
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// default CA certs for osquery. Copied from macOS's `/etc/ssl/cert.pem`
//
//go:embed ca-bundle.crt
var defaultCaCerts []byte

// InstallCaCerts returns the path to CA certificates.
// On Windows, it verifies SystemCertPool is available but uses embedded bundle for the file.
func InstallCaCerts(directory string) (string, error) {
	// Check if SystemCertPool is available (this validates Windows cert store access)
	if _, err := x509.SystemCertPool(); err != nil {
		// If SystemCertPool fails, log it but continue with embedded certs
		fmt.Printf("Warning: Windows system certificate pool unavailable: %v\n", err)
	}

	// Always use embedded bundle for file-based interface
	// Applications can use x509.SystemCertPool() directly if they prefer
	sum := sha256.Sum256(defaultCaCerts)
	caFile := filepath.Join(directory, fmt.Sprintf("ca-certs-embedded-%x.crt", sum))

	_, err := os.Stat(caFile)
	switch {
	case os.IsNotExist(err):
		return caFile, os.WriteFile(caFile, defaultCaCerts, 0444)
	case err != nil:
		return "", err
	}
	return caFile, nil
}

// GetSystemCertPool returns the Windows system certificate pool directly
// This is the preferred method for Windows applications using Go 1.18+
func GetSystemCertPool() (*x509.CertPool, error) {
	return x509.SystemCertPool()
}
