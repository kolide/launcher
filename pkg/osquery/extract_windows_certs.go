//go:build windows
// +build windows

package osquery

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
)

var (
	crypt32                     = syscall.NewLazyDLL("crypt32.dll")
	certOpenSystemStore         = crypt32.NewProc("CertOpenSystemStoreW")
	certCloseStore              = crypt32.NewProc("CertCloseStore")
	certEnumCertificatesInStore = crypt32.NewProc("CertEnumCertificatesInStore")
)

const (
	CERT_STORE_PROV_SYSTEM      = 0x0000000A
	CERT_CLOSE_STORE_FORCE_FLAG = 0x00000001
)

type CERT_CONTEXT struct {
	EncodingType uint32
	EncodedCert  *byte
	Length       uint32
	CertInfo     uintptr
	Store        uintptr
}

// extractSystemCerts extracts CA certificates from Windows system stores
func extractSystemCerts(slogger *slog.Logger) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate

	// Certificate store names to check
	storeNames := []string{"ROOT", "CA"}

	for _, storeName := range storeNames {
		storeCerts, err := extractCertsFromStore(storeName)
		if err != nil {
			// Log error but continue with other stores
			slogger.Log(context.TODO(), slog.LevelWarn,
				"failed to extract certificates from store",
				"store_name", storeName,
				"err", err,
			)
			continue
		}
		certs = append(certs, storeCerts...)
	}

	if len(certs) == 0 {
		return nil, errors.New("no certificates found in system stores")
	}

	return certs, nil
}

// extractCertsFromStore extracts certificates from a specific Windows certificate store
func extractCertsFromStore(storeName string) ([]*x509.Certificate, error) {
	// Convert store name to UTF16
	storeNamePtr, err := syscall.UTF16PtrFromString(storeName)
	if err != nil {
		return nil, fmt.Errorf("failed to convert store name: %w", err)
	}

	// Open the certificate store
	// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certopensystemstorew first argument is not used and should be set to 0.
	store, _, err := certOpenSystemStore.Call(0, uintptr(unsafe.Pointer(storeNamePtr)))
	if store == 0 {
		return nil, fmt.Errorf("failed to open certificate store %s: %w", storeName, err)
	}
	defer certCloseStore.Call(store, CERT_CLOSE_STORE_FORCE_FLAG)

	var certs []*x509.Certificate
	var prevContext uintptr

	for {
		// The CertEnumCertificatesInStore function retrieves the first or next certificate in a certificate store. Used in a loop, this function can retrieve in sequence all certificates in a certificate store.
		// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certenumcertificatesinstore
		context, _, _ := certEnumCertificatesInStore.Call(store, prevContext)
		if context == 0 {
			break
		}

		// Convert CERT_CONTEXT to Go struct
		certContext := (*CERT_CONTEXT)(unsafe.Pointer(context))

		// Create byte slice from certificate data
		certData := make([]byte, certContext.Length)
		for i := uint32(0); i < certContext.Length; i++ {
			certData[i] = *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(certContext.EncodedCert)) + uintptr(i)))
		}

		// Parse the certificate
		cert, err := x509.ParseCertificate(certData)
		if err != nil {
			// Skip invalid certificates
			prevContext = context
			continue
		}

		// Only include CA certificates
		if cert.IsCA {
			certs = append(certs, cert)
		}

		prevContext = context
	}

	return certs, nil
}
