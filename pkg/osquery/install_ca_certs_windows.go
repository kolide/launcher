//go:build windows

package osquery

import (
	"encoding/pem"
	"fmt"
	"log/slog"
)

// readSystemCaCerts returns the CA certificates in the Windows certificate store as a PEM bundle.
func readSystemCaCerts(slogger *slog.Logger) ([]byte, error) {
	certs, err := extractSystemCerts(slogger)
	if err != nil {
		return nil, fmt.Errorf("extracting system certificates: %w", err)
	}

	var pemData []byte
	for _, cert := range certs {
		pemData = append(pemData, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})...)
	}

	return pemData, nil
}
