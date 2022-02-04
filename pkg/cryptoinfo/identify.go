// Package cryptoinfo is designed to examine keys and certificates on
// disk, and return information about them. It is designed to work
// with dataflatten, and may eventually it may replace pkg/keyidentifier
package cryptoinfo

import (
	"bytes"

	"github.com/go-kit/kit/log"
)

var (
	certificateLeadingBytes = []byte{0x30} // used to detect raw DER certs
	//pkcs1LeadingBytes       = nil
	//pkcs8LeadingBytes       = nil
	//pkcs12LeadingBytes      = nil
)


// Identify examines a []byte and attempts to descern what
// cryptographic material is contained within.
func Identify(logger log.Logger, data []byte) ([]*KeyInfo, error) {
	switch {
	case bytes.HasPrefix(data, certificateLeadingBytes):
		return []*KeyInfo{expandDer(data)}, nil
	default:
		return decodePem(logger, data)
	}
}

func expandDer(data []byte) *KeyInfo {
	ki := NewKeyInfo(kiDER, kiCertificate, nil)
	ki.SetDataName("certificate")
	ki.SetData(parseCertificate(data))
	return ki
}
