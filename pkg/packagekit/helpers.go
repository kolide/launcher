package packagekit

import (
	"os"

	"github.com/pkg/errors"
)

func isDirectory(d string) error {
	if dStat, err := os.Stat(d); os.IsNotExist(err) {
		return errors.Wrapf(err, "missing packageRoot %s", d)
	} else {
		if !dStat.IsDir() {
			return errors.Errorf("packageRoot (%s) isn't a directory", d)
		}
	}
	return nil
}
