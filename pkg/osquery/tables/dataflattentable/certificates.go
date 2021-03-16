package dataflattentable

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"net"
	"net/url"
	"time"

	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/pkg/errors"
)

type certExtract struct {
	CRLDistributionPoints       []string
	DNSNames                    []string
	EmailAddresses              []string
	ExcludedDNSDomains          []string
	ExcludedEmailAddresses      []string
	ExcludedIPRanges            []*net.IPNet
	ExcludedURIDomains          []string
	IPAddresses                 []net.IP
	IssuerRaw                   pkix.Name
	Issuer                      string
	IssuingCertificateURL       []string
	KeyUsage                    []string
	NotBefore, NotAfter         time.Time
	OCSPServer                  []string
	PermittedDNSDomains         []string
	PermittedDNSDomainsCritical bool
	PermittedEmailAddresses     []string
	PermittedIPRanges           []*net.IPNet
	PermittedURIDomains         []string
	PublicKeyAlgorithm          string
	SerialNumber                string
	SignatureAlgorithm          string
	SubjectRaw                  pkix.Name
	Subject                     string
	URIs                        []*url.URL
	Version                     int
}

// flattenCertificate reads a certificate at path, and returns a
// flattened form. It's suitable for handing to the generalized table
// flattener.
func flattenCertificate(certpath string, _opts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
	certBytes, err := ioutil.ReadFile(certpath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading %s", certpath)
	}

	block, _ := pem.Decode(certBytes)
	if block == nil {
		return nil, errors.Errorf("Unable to read pem from %s", certpath)
	}

	rawCerts, err := x509.ParseCertificates(block.Bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "x509 parsing %s", certpath)
	}

	certs := make([]certExtract, len(rawCerts))
	for i, c := range rawCerts {
		certs[i] = certExtract{
			CRLDistributionPoints: c.CRLDistributionPoints,
			DNSNames:              c.DNSNames,
			EmailAddresses:        c.EmailAddresses,
			IPAddresses:           c.IPAddresses,
			IssuerRaw:             c.Issuer,
			Issuer:                c.Issuer.String(),
			IssuingCertificateURL: c.IssuingCertificateURL,
			KeyUsage:              keyUsageToStrings(c.KeyUsage),
			NotAfter:              c.NotAfter,
			NotBefore:             c.NotBefore,
			OCSPServer:            c.OCSPServer,
			PublicKeyAlgorithm:    c.PublicKeyAlgorithm.String(),
			SerialNumber:          c.SerialNumber.String(),
			SignatureAlgorithm:    c.SignatureAlgorithm.String(),
			SubjectRaw:            c.Subject,
			Subject:               c.Subject.String(),
			URIs:                  c.URIs,
			Version:               c.Version,
		}
	}

	// Bounce through json, because it's the simplest way to marshal the deep nested things like Subject
	certsJson, err := json.Marshal(certs)
	if err != nil {
		return nil, errors.Wrap(err, "json marshal")
	}

	rows, err := dataflatten.Json(certsJson)
	if err != nil {
		return nil, errors.Wrap(err, "flatten")
	}

	return rows, nil

}

var keyUsageBits = map[x509.KeyUsage]string{
	x509.KeyUsageContentCommitment: "Content Commitment",
	x509.KeyUsageKeyEncipherment:   "Key Encipherment",
	x509.KeyUsageDataEncipherment:  "Data Encipherment",
	x509.KeyUsageKeyAgreement:      "Key Agreement",
	x509.KeyUsageCertSign:          "Certificate Sign",
	x509.KeyUsageCRLSign:           "CRL Sign",
	x509.KeyUsageEncipherOnly:      "Encipher Only",
	x509.KeyUsageDecipherOnly:      "Decipher Only",
}

func keyUsageToStrings(k x509.KeyUsage) []string {
	var usage []string

	for usageBit, usageMeaning := range keyUsageBits {
		if k&usageBit != 0 {
			usage = append(usage, usageMeaning)
		}
	}

	return usage
}
