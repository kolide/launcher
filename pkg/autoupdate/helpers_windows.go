// +build windows

package autoupdate

import (
	"os"

	"github.com/pkg/errors"
)

// checkExecutable checks wehether a specific file looks like it's
// executable. This is used in evaluating whether something is an
// updated version.
func checkExecutable(potentialBinary string) error {
	if potentialBinary == "" {
		return errors.New("empty string isn't executable")
	}
	stat, err := os.Stat(potentialBinary)
	switch {
	case os.IsNotExist(err):
		return errors.Errorf("No such file %s", potentialBinary)
	case stat.IsDir():
		return errors.Errorf("%s is a directory", potentialBinary)
	case err != nil:
		return errors.Wrapf(err, "statting file %s", potentialBinary)
	}

	return nil
}
