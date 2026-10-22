package osquery

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
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

	// Generated with a random key, so build it once to use in both the bundle and the expected output.
	sameSubjectNewKey := sameSubjectNewKeyCert(t, emSignRootCsG3)

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
		{
			name:     "unparseable certificate blocks are preserved",
			bundle:   trusted[0] + "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n" + emSignEccRootG3,
			expected: trusted[0] + "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n",
		},
		{
			name:     "re-issued root with the same key but different serial and validity is removed",
			bundle:   trusted[0] + reissuedCert(t, emSignEccRootG3) + trusted[1],
			expected: trusted[0] + trusted[1],
		},
		{
			name:     "cross-certificate for a distrusted key is removed",
			bundle:   trusted[0] + crossCert(t, emSignRootTlsG3) + trusted[1],
			expected: trusted[0] + trusted[1],
		},
		{
			name:     "certificate with a distrusted subject but a different key is preserved",
			bundle:   trusted[0] + sameSubjectNewKey + trusted[1],
			expected: trusted[0] + sameSubjectNewKey + trusted[1],
		},
		{
			name:     "distrusted TRUSTED CERTIFICATE block is removed",
			bundle:   trusted[0] + trustedCertBlock(t, emSignEccRootG3) + trusted[1],
			expected: trusted[0] + trusted[1],
		},
		{
			name:     "distrusted TRUSTED CERTIFICATE block with no trust settings is removed",
			bundle:   trusted[0] + retypedBlock(t, "TRUSTED CERTIFICATE", emSignRootTlsG3) + trusted[1],
			expected: trusted[0] + trusted[1],
		},
		{
			name:     "distrusted X509 CERTIFICATE block is removed",
			bundle:   trusted[0] + retypedBlock(t, "X509 CERTIFICATE", emSignRootCsG3) + trusted[1],
			expected: trusted[0] + trusted[1],
		},
		{
			name:     "trusted TRUSTED CERTIFICATE block is preserved",
			bundle:   trustedCertBlock(t, trusted[0]) + emSignEccRootG3 + trusted[1],
			expected: trustedCertBlock(t, trusted[0]) + trusted[1],
		},
		{
			name:     "trusted X509 CERTIFICATE block is preserved",
			bundle:   retypedBlock(t, "X509 CERTIFICATE", trusted[0]) + emSignEccRootG3,
			expected: retypedBlock(t, "X509 CERTIFICATE", trusted[0]),
		},
		{
			name:     "TRUSTED CERTIFICATE block with unparseable contents is preserved",
			bundle:   trusted[0] + "-----BEGIN TRUSTED CERTIFICATE-----\nAAAA\n-----END TRUSTED CERTIFICATE-----\n" + emSignEccRootG3,
			expected: trusted[0] + "-----BEGIN TRUSTED CERTIFICATE-----\nAAAA\n-----END TRUSTED CERTIFICATE-----\n",
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

// parseCert parses a single PEM-encoded certificate.
func parseCert(t *testing.T, pemCert string) *x509.Certificate {
	block, _ := pem.Decode([]byte(pemCert))
	require.NotNil(t, block, "decoding PEM")
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "parsing certificate")
	return cert
}

// testSigningKey returns a throwaway key for signing test certificates. The resulting signatures
// will not verify against the embedded public key, but filterCaCerts does not check signatures,
// and neither does a system trust store when deciding whether to hand us a root.
func testSigningKey(t *testing.T) *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err, "generating signing key")
	return key
}

// encodeCert builds a certificate from template, embedding pub as its public key, and returns it
// PEM-encoded. If issuer is nil, the certificate is self-issued.
func encodeCert(t *testing.T, template *x509.Certificate, issuer *x509.Certificate, pub any) string {
	if issuer == nil {
		issuer = template
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, pub, testSigningKey(t))
	require.NoError(t, err, "creating certificate")
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// reissuedCert returns a self-issued certificate with the same subject and public key as
// original, but a different serial number and validity period -- the shape of a root that a
// CA re-issues to extend its lifetime.
func reissuedCert(t *testing.T, original string) string {
	orig := parseCert(t, original)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(0xdecaf),
		Subject:               orig.Subject,
		NotBefore:             orig.NotBefore.AddDate(5, 0, 0),
		NotAfter:              orig.NotAfter.AddDate(5, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	return encodeCert(t, template, nil, orig.PublicKey)
}

// crossCert returns a certificate for original's subject and public key, issued by a different,
// unrelated CA -- the shape of a cross-signed root.
func crossCert(t *testing.T, original string) string {
	orig := parseCert(t, original)
	issuer := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Some Other Root CA"},
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(0xc0ffee),
		Subject:               orig.Subject,
		NotBefore:             orig.NotBefore,
		NotAfter:              orig.NotAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	return encodeCert(t, template, issuer, orig.PublicKey)
}

// sameSubjectNewKeyCert returns a self-issued certificate with the same subject and validity as
// original but a freshly generated key. It must not be filtered: only the key is distrusted, not
// the name.
func sameSubjectNewKeyCert(t *testing.T, original string) string {
	orig := parseCert(t, original)
	template := &x509.Certificate{
		SerialNumber:          orig.SerialNumber,
		Subject:               orig.Subject,
		NotBefore:             orig.NotBefore,
		NotAfter:              orig.NotAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	return encodeCert(t, template, nil, &testSigningKey(t).PublicKey)
}

// retypedBlock re-encodes the single PEM block in pemCert with the given block type, leaving the
// contents unchanged.
func retypedBlock(t *testing.T, blockType string, pemCert string) string {
	block, _ := pem.Decode([]byte(pemCert))
	require.NotNil(t, block, "decoding PEM")
	return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: block.Bytes}))
}

// trustedCertBlock returns pemCert as an OpenSSL TRUSTED CERTIFICATE block: the certificate DER
// followed by X509_CERT_AUX trust settings, as written by `openssl x509 -trustout -addtrust
// serverAuth -setalias ...`.
func trustedCertBlock(t *testing.T, pemCert string) string {
	block, _ := pem.Decode([]byte(pemCert))
	require.NotNil(t, block, "decoding PEM")

	// X509_CERT_AUX ::= SEQUENCE { trust SEQUENCE OF OBJECT IDENTIFIER OPTIONAL, ..., alias UTF8String OPTIONAL, ... }
	aux, err := asn1.Marshal(struct {
		Trust []asn1.ObjectIdentifier
		Alias string `asn1:"utf8"`
	}{
		Trust: []asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 1}}, // id-kp-serverAuth
		Alias: "test alias",
	})
	require.NoError(t, err, "marshalling X509_CERT_AUX")

	return string(pem.EncodeToMemory(&pem.Block{Type: "TRUSTED CERTIFICATE", Bytes: append(block.Bytes, aux...)}))
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
