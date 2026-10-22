package osquery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/asn1"
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

// distrustedCaKeyHashes lists SHA-256 hashes of the SubjectPublicKeyInfo of each CA key that must
// never be handed to osquery, even when the system trust store still contains a certificate for
// it. Note that we do not use the remote feature of osquery (https://osquery.readthedocs.io/en/latest/deployment/remote/);
// the usage of these certs for us is limited to e.g. the `curl` table.
//
// The four roots and eight intermediate keys below are taken from https://emudhra.we-have-a-problem.com/roots.json
// and https://emudhra.we-have-a-problem.com/impacted-intermediates.json.
//
// Matching on the key rather than on the whole certificate catches re-issued roots, cross-signs,
// and alternate encodings, none of which share a whole-certificate fingerprint with the original.
// The intermediates are listed as well as the roots because on Windows we export the CA store,
// which holds intermediates, and osquery treats every certificate in its bundle as a trust anchor.
var distrustedCaKeyHashes = map[string]string{
	// Roots
	"8d417db2dd8bf5e3084d1e3f196d583849d81bdd4c00c70b9d39369e96b8c782": "emSign ECC Root CA - G3",
	"c619e4780ed8e0abeda24ad732567de4383a7ea941338f6d1a7d1b49ac792fe9": "emSign Root TLS CA - G3",
	"0a6c2b2ca5b364207954171987ac9d2178561569d8205457e77ca90c32ede282": "emSign Root CS CA - G3",
	"bd153ed7b0434f6886b17bce8bbe84ed340c7132d702a8f4fa318f756ecbd6f3": "AAA Certificate Services",
	// Intermediates
	"ebed6475c2c2d80fcb199f3fe8dd6e6b3ce60b77eb15d4e42c3dec0c6f5b5105": "EM DV TLS CA - G3B",
	"43bffa19d6ef651451f6841e019931e96f6f2eaeff28ab4b6f1ce4576de2717b": "EM OV TLS CA - G3B",
	"e0254c7fcdc265e2f0f8a428dabb75ab4ae039b4872d62f5ee57f0877d5a8a41": "EM EV TLS CA - G3B",
	"49dd353dc2b919a3770645117d0e60c16115e7d1bc259eefe734b00ea434fe5f": "EM DV TLS CA - G3C",
	"63ecefee431d49523bc1136c4e72d0206a477086caf606cd5cf025686c109cf8": "EM OV TLS CA - G3C",
	"5c0660a2b164910d8ff589c18e14552e9e0ec237dcefe2cb736d41a4811f81a0": "EM EV TLS CA - G3C",
	"ec4ffa1ad0ac88081314b7aaffef6a91f97cd57291bb5ba686c5415f5eb4f6b6": "EM OV CS CA - G3B",
	"92c063d67e8f14db71d288e11266ffdaf69cbf115f7bab14f3bc2f68d25e0e30": "EM OV CS CA - G3C",
}

// InstallCaCerts returns the path to CA certificates for osquery. It prefers the system CA
// bundle over the embedded one so that customers can supply their own CAs. Either way, the
// bundle is written out with distrusted roots removed. Because the filtered bundle is a copy
// taken when osquery starts, any CAs added to the system bundle afterward will not be picked
// up until osquery restarts.
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

	// Reuse an existing file only if its contents are byte-for-byte what we would write.
	existing, err := os.ReadFile(caFile)
	switch {
	case err == nil && bytes.Equal(existing, filtered):
		return caFile, nil
	case err == nil:
		// Removing first (rather than renaming over it) handles Windows, where a rename onto a
		// read-only file fails; os.Remove clears the read-only attribute there.
		if err := os.Remove(caFile); err != nil {
			return "", fmt.Errorf("removing CA certs file with unexpected contents: %w", err)
		}
	case !os.IsNotExist(err):
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
		// derived from the content, so if it exists now with the expected contents, it is
		// the file we wanted.
		if existing, readErr := os.ReadFile(caFile); readErr == nil && bytes.Equal(existing, filtered) {
			return caFile, nil
		}
		return "", fmt.Errorf("renaming temp CA certs file into place: %w", err)
	}

	return caFile, nil
}

var certificatePemTypes = map[string]struct{}{
	"CERTIFICATE":         {},
	"X509 CERTIFICATE":    {},
	"TRUSTED CERTIFICATE": {},
}

// filterCaCerts returns pemBundle with any distrusted certificates removed. Everything else,
// including non-certificate PEM blocks and any text between blocks, is passed through unchanged.
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
		if _, isCert := certificatePemTypes[block.Type]; !isCert || !isDistrustedCaCert(leadingCertificate(block.Bytes)) {
			out = append(out, consumed...)
		}

		rest = remaining
	}
}

// leadingCertificate returns the first ASN.1 element in der, dropping anything after it. For a
// plain certificate this is the whole input; for a TRUSTED CERTIFICATE it strips the trailing
// trust settings so that the certificate can be parsed. If der does not start
// with a well-formed ASN.1 element, it is returned unchanged.
func leadingCertificate(der []byte) []byte {
	var raw asn1.RawValue
	if _, err := asn1.Unmarshal(der, &raw); err != nil {
		return der
	}
	return raw.FullBytes
}

// isDistrustedCaCert reports whether the DER-encoded certificate is one we must not hand to
// osquery, i.e. whether its public key is in distrustedCaKeyHashes.
func isDistrustedCaCert(der []byte) bool {
	// System bundles can contain legacy certs that Go's x509 package rejects (e.g. negative serial
	// numbers). Those cannot be matched by key, so they are passed through. All of the distrusted
	// certificates parse cleanly.
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return false
	}

	keyHash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	_, distrusted := distrustedCaKeyHashes[hex.EncodeToString(keyHash[:])]
	return distrusted
}
