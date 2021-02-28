// Package augeas provides some lenses for use with osquery's augeas
// support.
//
// As we run a subset of the lenses, the simplest way to check for
// missing dependancies is to augtool:
//    augtool -c -S  -I ./assets/lenses
//
// Also handy is seeing what files this will ingest:
//    augtool  -S  -I .//assets/lenses print /files//*
package augeas

import (
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
)

//go:generate go-bindata -nometadata -nocompress -pkg augeas -o assets.go assets/lenses

func InstallLenses(targetDir string) error {
	lenses, err := AssetDir("assets/lenses")
	if err != nil {
		return errors.Wrap(err, "listing lenses")
	}

	for _, lens := range lenses {
		lensBytes, err := Asset("assets/lenses/" + lens)
		if err != nil {
			return errors.Wrapf(err, "fetching lens %s", lens)
		}

		err = ioutil.WriteFile(filepath.Join(targetDir, lens), lensBytes, 0644)
		if err != nil {
			return errors.Wrapf(err, "writing lens %s", lens)
		}

	}
	return nil
}
