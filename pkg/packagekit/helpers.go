package packagekit

import (
	"os"

	"github.com/pkg/errors"
)

func isDirectory(d string) error {
	dStat, err := os.Stat(d)

	if os.IsNotExist(err) {
		return errors.Wrapf(err, "missing packageRoot %s", d)
	}

	if !dStat.IsDir() {
		return errors.Errorf("packageRoot (%s) isn't a directory", d)
	}

	return nil
}
