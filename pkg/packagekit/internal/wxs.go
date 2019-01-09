package internal

import "github.com/pkg/errors"

//go:generate go-bindata -pkg $GOPACKAGE -o assets.go assets/

func InstallWXS() ([]byte, error) {
	data, err := Asset("assets/installer.wxs")
	if err != nil {
		return nil, errors.Wrap(err, "getting go-bindata install.wxs")
	}
	return data, nil
}
