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

// Options
//
// To allow a common function signature, while still allowing our
// methods to take different options. And _still_ allowing those to be
// compiled with their types, we expand on the functional argument style.
// This is based:
//  https://derekchiang.com/posts/reusable-and-type-safe-options-for-go-apis/
//  https://gist.github.com/derekchiang/c9709eace4b588353ec9df32d845ac9c

// SigningKey Option
type signingKeyOption struct {
	v string
}

func WithSigningKey(v string) interface {
	PkgOption
} {
	return &signingKeyOption{
		v: v,
	}
}

func (o *signingKeyOption) SetPkgOption(opts *pkgOptions) {
	opts.SigningKey = o.v
}

/*
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
*/

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
