//go:build !windows

package osquery

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
)

// readSystemCaCerts returns the contents of the system CA bundle, checking well-known
// locations for the current OS.
func readSystemCaCerts(_ *slog.Logger) ([]byte, error) {
	var candidates []string

	switch runtime.GOOS {
	case "linux":
		candidates = []string{
			"/etc/ssl/certs/ca-certificates.crt",
			"/etc/pki/tls/certs/ca-bundle.crt",
			"/etc/ssl/ca-bundle.pem",
			"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
		}
	case "darwin":
		candidates = []string{
			"/etc/ssl/cert.pem",
			"/System/Library/OpenSSL/certs/cert.pem",
		}
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	for _, path := range candidates {
		if contents, err := os.ReadFile(path); err == nil {
			return contents, nil
		}
	}

	return nil, errors.New("no system CA certificates found")
}
