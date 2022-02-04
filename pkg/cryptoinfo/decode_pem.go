package cryptoinfo

import (
	"encoding/pem"

	"github.com/pkg/errors"
)

func decodePem(pemBytes []byte) ([]*KeyInfo, error) {
	expanded := []*KeyInfo{}

	// Loop over the bytes, reading pem blocks
	var block *pem.Block
	for len(pemBytes) > 0 {
		block, pemBytes = pem.Decode(pemBytes)
		if block == nil {
			// When pem.Decode finds no pem, it returns a nil block, and the input as rest.
			// In that case, we stop parsing, as anything else would land in an infinite loop
			break
		}

		expanded = append(expanded, expandPem(block))
	}

	return expanded, nil
}

func expandPem(block *pem.Block) *KeyInfo {
	ki := NewKeyInfo(kiPEM, block.Type, block.Headers)

	switch block.Type {
	case "CERTIFICATE":
		ki.SetDataName("certificate").SetData(parseCertificate(block.Bytes))
	default:
		ki.Error = errors.Errorf("Unknown block type: %s", block.Type)
	}

	return ki
}
