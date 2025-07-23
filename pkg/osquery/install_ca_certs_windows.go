//go:build windows
// +build windows

package osquery

import (
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// default CA certs for osquery. Copied from macOS's `/etc/ssl/cert.pem`
//
//go:embed ca-bundle.crt
var defaultCaCerts []byte

const (
	CRYPT_E_NOT_FOUND = 0x80092004
	maxEncodedCertLen = 1 << 20
)

// InstallCaCerts returns the path to CA certificates.
// On Windows, it extracts certificates from the system store and creates a bundle file.
func InstallCaCerts(directory string) (string, error) {
	// Try to extract and use system CA certificates
	systemCerts, err := extractWindowsSystemCerts()
	if err == nil && len(systemCerts) > 0 {
		// Create a hash of the system certs for caching
		certData := encodeCertificatesToPEM(systemCerts)
		sum := sha256.Sum256(certData)
		systemCaFile := filepath.Join(directory, fmt.Sprintf("ca-certs-system-%x.crt", sum))

		// Check if file already exists
		if _, err := os.Stat(systemCaFile); os.IsNotExist(err) {
			// Write system certificates to file
			if err := os.WriteFile(systemCaFile, certData, 0444); err != nil {
				// If we can't write system certs, fall back to embedded
				return installEmbeddedCerts(directory)
			}
		}
		return systemCaFile, nil
	}

	// Fall back to installing embedded bundle
	return installEmbeddedCerts(directory)
}

// installEmbeddedCerts installs the embedded certificate bundle
func installEmbeddedCerts(directory string) (string, error) {
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

// extractWindowsSystemCerts extracts certificates from Windows certificate store
func extractWindowsSystemCerts() ([]*x509.Certificate, error) {
	root, err := syscall.UTF16PtrFromString("Root")
	if err != nil {
		return nil, fmt.Errorf("unable to create UTF16 pointer: %v", err)
	}

	storeHandle, err := syscall.CertOpenSystemStore(0, root)
	if err != nil {
		return nil, fmt.Errorf("unable to open system certificate store: %v", err)
	}

	var certs []*x509.Certificate
	var certContext *syscall.CertContext

	defer func(store syscall.Handle, flags uint32) {
		_ = syscall.CertCloseStore(store, flags)
	}(storeHandle, 0)

	for {
		certContext, err = syscall.CertEnumCertificatesInStore(storeHandle, certContext)
		if err != nil {
			if errno, ok := err.(syscall.Errno); ok {
				if errno == CRYPT_E_NOT_FOUND {
					break // No more certificates
				}
			}
			return nil, fmt.Errorf("unable to enumerate certificates: %v", err)
		}

		if certContext == nil {
			break
		}

		if certContext.Length > maxEncodedCertLen {
			return nil, fmt.Errorf("invalid certificate context length %d", certContext.Length)
		}

		buf := (*[maxEncodedCertLen]byte)(unsafe.Pointer(certContext.EncodedCert))[:certContext.Length]

		cert, err := x509.ParseCertificate(buf)
		if err != nil {
			continue // Skip invalid certificates
		}

		certs = append(certs, cert)
	}

	return certs, nil
}

// encodeCertificatesToPEM converts x509 certificates to PEM format bundle
func encodeCertificatesToPEM(certs []*x509.Certificate) []byte {
	var pemData []byte

	for _, cert := range certs {
		pemBlock := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		}
		pemData = append(pemData, pem.EncodeToMemory(pemBlock)...)
	}

	return pemData
}
