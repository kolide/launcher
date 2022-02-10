// Package cryptoinfo is designed to examine keys and certificates on
// disk, and return information about them. It is designed to work
// with dataflatten, and may eventually it may replace pkg/keyidentifier
package cryptoinfo

type identifierSigfunc func(data []byte, password string) (results []*KeyInfo, err error)

var identifiers = []identifierSigfunc{
	tryP12,
	tryDer,
	tryPem,
}

// Identify examines a []byte and attempts to descern what
// cryptographic material is contained within.
func Identify(data []byte, password string) ([]*KeyInfo, error) {
	for _, fn := range identifiers {
		res, err := fn(data, password)
		if err == nil {
			return res, nil
		}
	}

	// If we can't parse anything, return nothing. It's not a fatal error, and it's
	// somewhart obvious from context that nothing was parsed. q
	return nil, nil
}

func tryDer(data []byte, _password string) ([]*KeyInfo, error) {
	cert, err := parseCertificate(data)
	if err != nil {
		return nil, err
	}

	return []*KeyInfo{NewKICertificate(kiDER).SetData(cert, err)}, nil
}
