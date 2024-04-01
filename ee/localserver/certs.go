package localserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// These are the hardcoded certificates
const (
	k2RsaServerCert = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAkeNJgRkJOow7LovGmrlW
1UzHkifTKQV1/8kX+p2MPLptGgPKlqpLnhZsGOhpHpswlUalgSZPyhBfM9Btdmps
QZ2PkZkgEiy62PleVSBeBtpGcwHibHTGamzmKVrji9GudAvU+qapfPGnr//275/1
E+mTriB5XBrHic11YmtCG6yg0Vw383n428pNF8QD/Bx8pzgkie2xKi/cHkc9B0S2
B2rdYyWP17o+blgEM+EgjukLouX6VYkbMYhkDcy6bcUYfknII/T84kuChHkuWyO5
msGeD7hPhtdB/h0O8eBWIiOQ6fH7exl71UfGTR6pYQmJMK1ZZeT7FeWVSGkswxkV
4QIDAQAB
-----END PUBLIC KEY-----
`

	reviewRsaServerCert = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAr0VjwKya7JNRM8uiPllw
An+W3SBuDkxAToqHxdRX6k2eJSIK0K4oynVIqkrP1MC2ultlIo2ZZhKYQVhQfCej
9RIBFm2wl1/daMNCpmkwu8KbsXDAVrc70yXvpzeAnh6QCnvI1PbCI6icbpVo8Wh1
6D2SBvJEe8Ag0mjxC4GLMhhaLSgFOIY3F7ts2oMECbd3icf5vdJ1aJy/W0bW2AVb
tZNE1czioSOXzCnQ08BqmL9aPL9l4u/cmCesNeTaHENDlnkWhiG4I/BeJwpbZElP
usDCeod74rofJNTl9juqJNUW4S5QjCxJiZ+ZKTiKWu/vY7xjr7dZgQ1+97oXpTqt
AwIDAQAB
-----END PUBLIC KEY-----
`

	localhostRsaServerCert = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAwXIB33Wvu/rn7WjSQaat
9lafwrrnmbP8NtTlOiY4b4gv/bL6nnyr21s95uKlcm+8WRbJZsch/ahrNYsdDO2Q
QmfZTi7VR7/IhwyISkh/JaaBPmipO/4KfdnKOarah3F619fl4Udd973+5QK0ZQmy
eg9sJ4UKs/QUI23Nv9uXL6WBY8SiXjmcp37aAvs2mm9Tk2ar6dFyLBTSM+TzC9u8
MWKCWC4QZMV2iPy1GDj+IwujAKPzAl2VGvGL+HuoeOuwKR7nluMgkd5FWf3m9qS3
Skx1Y1JUHgZL9IVGMAmkJWEKoa4TPopfnr74SwpNDcU7rP86rgSIO597wMeMbnAM
8wIDAQAB
-----END PUBLIC KEY-----
`

	k2EccServerCert = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEmAO4tYINU14/i0LvONs1IXVwaFnF
dNsydDr38XrL29kiFl+vTkp4gVx6172oNSL3KRBQmjMXqWkLNoxXaWS3uQ==
-----END PUBLIC KEY-----`

	reviewEccServerCert = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIgYTWPi8N7b0H69tnN543HbjAoLc
GINysvEwYrNoGjASt+nqzlFesagt+2A/4W7JR16nE91mbCHn+HV6x+H8gw==
-----END PUBLIC KEY-----`

	localhostEccServerCert = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEwowFsPUaOC61LAfDz1hLnsuSDfEx
SC4TSfHtbHHv3lx2/Bfu+H0szXYZ75GF/qZ5edobq3UkABN6OaFnnJId3w==
-----END PUBLIC KEY-----`
)

func generateSelfSignedCert(_ context.Context) (tls.Certificate, error) {
	// Generate a random serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generating serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Kolide, Inc"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour), // approximately 3 months
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
		},
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generating private key: %w", err)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("creating certificate: %w", err)
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return tls.Certificate{}, fmt.Errorf("encoding cert to pem: %w", err)
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshalling private key: %w", err)
	}

	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return tls.Certificate{}, fmt.Errorf("encoding key to pem: %w", err)
	}

	certBytes := certBuf.Bytes()

	cert, err := tls.X509KeyPair(certBytes, keyBuf.Bytes())
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("loading key pair: %w", err)
	}

	/*
		x509Cert, err := x509.ParseCertificate(derBytes)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("parsing cert: %w", err)
		}

		if err := addCertToKeyStore(ctx, certBytes, x509Cert); err != nil {
			return tls.Certificate{}, fmt.Errorf("adding cert to key store: %w", err)
		}
	*/

	return cert, nil
}
