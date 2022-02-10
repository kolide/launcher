package cryptoinfo

func tryDer(data []byte, _password string) ([]*KeyInfo, error) {
	cert, err := parseCertificate(data)
	if err != nil {
		return nil, err
	}

	return []*KeyInfo{NewKICertificate(kiDER).SetData(cert, err)}, nil
}
