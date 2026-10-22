package osquery

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

// Distrusted roots, copied from https://emudhra.we-have-a-problem.com/roots.json.
const (
	emSignEccRootG3 = `-----BEGIN CERTIFICATE-----
MIICTjCCAdOgAwIBAgIKPPYHqWhwDtqLhDAKBggqhkjOPQQDAzBrMQswCQYDVQQG
EwJJTjETMBEGA1UECxMKZW1TaWduIFBLSTElMCMGA1UEChMcZU11ZGhyYSBUZWNo
bm9sb2dpZXMgTGltaXRlZDEgMB4GA1UEAxMXZW1TaWduIEVDQyBSb290IENBIC0g
RzMwHhcNMTgwMjE4MTgzMDAwWhcNNDMwMjE4MTgzMDAwWjBrMQswCQYDVQQGEwJJ
TjETMBEGA1UECxMKZW1TaWduIFBLSTElMCMGA1UEChMcZU11ZGhyYSBUZWNobm9s
b2dpZXMgTGltaXRlZDEgMB4GA1UEAxMXZW1TaWduIEVDQyBSb290IENBIC0gRzMw
djAQBgcqhkjOPQIBBgUrgQQAIgNiAAQjpQy4LRL1KPOxst3iAhKAnjlfSU2fySU0
WXTsuwYc58Byr+iuL+FBVIcUqEqy6HyC5ltqtdyzdc6LBtCGI79G1Y4PPwT01xyS
fvalY8L1X44uT6EYGQIrMgqCZH0Wk9GjQjBAMB0GA1UdDgQWBBR8XQKEE9TMipuB
zhccLikenEhjQjAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAKBggq
hkjOPQQDAwNpADBmAjEAvvNhzwIQHWSVB7gYboiFBS+DCBeQyh+KTOgNG3qxrdWB
CUfvO6wIBHxcmbHtRwfSAjEAnbpV/KlK6O3t5nYBQnvI+GDZjVGLVTv7jHvrZQnD
+JbNR6iC8hZVdyR+EhCVBCyj
-----END CERTIFICATE-----
`
	emSignRootTlsG3 = `-----BEGIN CERTIFICATE-----
MIICKTCCAa+gAwIBAgIQDnYGcvFDRZ/I/gqwvAXjlDAKBggqhkjOPQQDAzBWMQsw
CQYDVQQGEwJJTjElMCMGA1UECgwcZU11ZGhyYSBUZWNobm9sb2dpZXMgTGltaXRl
ZDEgMB4GA1UEAwwXZW1TaWduIFJvb3QgVExTIENBIC0gRzMwHhcNMjQwMjA4MDgx
OTMyWhcNNDkwMjA3MDgxOTMyWjBWMQswCQYDVQQGEwJJTjElMCMGA1UECgwcZU11
ZGhyYSBUZWNobm9sb2dpZXMgTGltaXRlZDEgMB4GA1UEAwwXZW1TaWduIFJvb3Qg
VExTIENBIC0gRzMwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAARfD9w/q1cWxL6D9bMG
H6CQ4u+gAV6GN9s+PwQx9Bl4hC+0iU8QOk8mWO/RwXQy10DCSxT8vSIJClHKp+Ao
4qJKdYeV0SwZ7LMQziyLfxIUOY6Grja/sQFwLKYf8EdDXAyjQjBAMB0GA1UdDgQW
BBThOqshfPHjTVSu1HK3/YtZQiCQVDAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/
BAUwAwEB/zAKBggqhkjOPQQDAwNoADBlAjBhCFTkly7Yl7+s4hiw+k6ZJTXT2AiI
rvmsmfqPr5E7ecwXb6kXA5LTFd6rjwTuV3sCMQC9w6ZSbVq1mveMO6dKFziOzClT
KCT2RFT6yXtq6nzgRICgBmiXbJAXsmZ8yAwjyWc=
-----END CERTIFICATE-----
`
	emSignRootCsG3 = `-----BEGIN CERTIFICATE-----
MIICKDCCAa2gAwIBAgIQAY/OySfBFvWcmX7cDM0uKzAKBggqhkjOPQQDAzBVMQsw
CQYDVQQGEwJJTjElMCMGA1UECgwcZU11ZGhyYSBUZWNobm9sb2dpZXMgTGltaXRl
ZDEfMB0GA1UEAwwWZW1TaWduIFJvb3QgQ1MgQ0EgLSBHMzAeFw0yNDAyMDgwODMz
MTRaFw00OTAyMDcwODMzMTRaMFUxCzAJBgNVBAYTAklOMSUwIwYDVQQKDBxlTXVk
aHJhIFRlY2hub2xvZ2llcyBMaW1pdGVkMR8wHQYDVQQDDBZlbVNpZ24gUm9vdCBD
UyBDQSAtIEczMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAESl9kM5D0VA/Og76dz5Xz
w1hnGQ4fSR9993/qBkkvI48pXQVKOdyHCgAZThgWejQ8OpFRiTISjpabn4tfRFOY
uzhApR/XxGbkg9X+2cEtbyghF9CdoF91W22V/R2EVe4xo0IwQDAdBgNVHQ4EFgQU
aHQD+IXKjVRQVx0W4MjeRQ2rzS0wDgYDVR0PAQH/BAQDAgEGMA8GA1UdEwEB/wQF
MAMBAf8wCgYIKoZIzj0EAwMDaQAwZgIxAK9OlACrFod3UgWY1ai+DF9sGFhahU59
rv4jJyV01QaxGn0HErZ8gjHkngqe+tcFogIxAPTdW0hBmrGjhxBWdv9mZNdVFiJ4
lrZ3ahvimsLhTQpnDBgiGKv6Ov1Fx/CE6yjIAg==
-----END CERTIFICATE-----
`
)

func TestInstallCaCerts(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	slogger := multislogger.NewNopLogger()

	installedPath1, err := InstallCaCerts(tempDir, slogger)
	require.NoError(t, err, "install certs one")

	installedPath2, err := InstallCaCerts(tempDir, slogger)
	require.NoError(t, err, "install certs two")
	require.Equal(t, installedPath1, installedPath2, "reinstalled file has the same path")

	installed, err := os.ReadFile(installedPath1)
	require.NoError(t, err)
	require.Equal(t, installed, filterCaCerts(installed), "installed bundle should not contain distrusted certs")

	certs := parseCerts(installed)
	require.NotEmpty(t, certs, "installed bundle should contain certs")
	for _, cert := range certs {
		require.NotEqual(t, "emSign ECC Root CA - G3", cert.Subject.CommonName)
	}
}

func Test_filterCaCerts(t *testing.T) {
	t.Parallel()

	// Take a few trusted roots from the embedded bundle to mix in with the distrusted ones.
	trusted := pemBlocks(defaultCaCerts)[:3]
	require.Len(t, trusted, 3)

	for _, tt := range []struct {
		name     string
		bundle   string
		expected string
	}{
		{
			name:     "empty bundle",
			bundle:   "",
			expected: "",
		},
		{
			name:     "trusted certs are untouched",
			bundle:   trusted[0] + trusted[1] + trusted[2],
			expected: trusted[0] + trusted[1] + trusted[2],
		},
		{
			name:     "distrusted cert is removed from the middle",
			bundle:   trusted[0] + emSignEccRootG3 + trusted[1],
			expected: trusted[0] + trusted[1],
		},
		{
			name:     "distrusted cert is removed from the start and end",
			bundle:   emSignRootTlsG3 + trusted[0] + emSignRootCsG3,
			expected: trusted[0],
		},
		{
			name:     "all distrusted certs are removed",
			bundle:   emSignEccRootG3 + emSignRootTlsG3 + emSignRootCsG3,
			expected: "",
		},
		{
			name:     "surrounding text is preserved, text preceding a distrusted cert is removed",
			bundle:   "## header\n" + trusted[0] + "\n=== emSign ECC Root CA - G3\n" + emSignEccRootG3 + "\n=== trusted\n" + trusted[1] + "## trailer\n",
			expected: "## header\n" + trusted[0] + "\n=== trusted\n" + trusted[1] + "## trailer\n",
		},
		{
			name:     "non-certificate blocks are preserved",
			bundle:   trusted[0] + "-----BEGIN NOT A CERT-----\nAAAA\n-----END NOT A CERT-----\n" + emSignEccRootG3,
			expected: trusted[0] + "-----BEGIN NOT A CERT-----\nAAAA\n-----END NOT A CERT-----\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, string(filterCaCerts([]byte(tt.bundle))))
		})
	}
}

// Test_filterCaCerts_EmbeddedBundleIsClean confirms that our embedded bundle
// does not contain any of the known distrusted certs.
func Test_filterCaCerts_EmbeddedBundleIsClean(t *testing.T) {
	t.Parallel()

	require.True(t, bytes.Equal(defaultCaCerts, filterCaCerts(defaultCaCerts)), "embedded bundle should not contain distrusted certs")
}

// pemBlocks splits bundle into its PEM-encoded blocks, re-encoded without surrounding text.
func pemBlocks(bundle []byte) []string {
	var blocks []string
	for {
		block, rest := pem.Decode(bundle)
		if block == nil {
			return blocks
		}
		blocks = append(blocks, string(pem.EncodeToMemory(block)))
		bundle = rest
	}
}

// parseCerts parses the certificates in bundle, skipping any that Go's x509 package rejects
// (system bundles may contain legacy certs, e.g. with negative serial numbers).
func parseCerts(bundle []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for _, block := range pemBlocks(bundle) {
		b, _ := pem.Decode([]byte(block))
		if cert, err := x509.ParseCertificate(b.Bytes); err == nil {
			certs = append(certs, cert)
		}
	}
	return certs
}
