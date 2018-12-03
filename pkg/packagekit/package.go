package packagekit

import (
	"os"

	"github.com/pkg/errors"
)

type PackageOptions struct {
	Version      string
	AfterInstall string // postinstall script to run.
	Root         string // directory to package up
	Name         string // What's the name for this package
}

// Stuff below is probably crap
type Option func(*PackageOptions)

func WithVersion(v string) Option {
	return func(o *PackageOptions) {
		o.Version = v
	}
}

func WithAfterInstall(s string) Option {
	return func(o *PackageOptions) {
		o.AfterInstall = s
	}
}

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
