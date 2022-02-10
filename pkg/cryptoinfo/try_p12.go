package cryptoinfo

import p12 "software.sslmate.com/src/go-pkcs12"

func tryP12(data []byte, password string) ([]*KeyInfo, error) {
	privateKey, cert, caCerts, err := p12.DecodeChain(data, password)
	if err != nil {
		return nil, err
	}

	results := []*KeyInfo{}

	if privateKey != nil {
		results = append(results, NewKIKey(kiP12))
	}

	if cert != nil {
		results = append(results, NewKICertificate(kiP12).SetData(extractCert(cert)))
	}

	for _, c := range caCerts {
		results = append(results, NewKICaCertificate(kiP12).SetData(extractCert(c)))
	}

	return results, nil
}
