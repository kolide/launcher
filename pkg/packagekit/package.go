package packagekit

import (
	"io"
)

// PackageOptions is the superset of all packaging options. Not all
// packages will support all options.
type PackageOptions struct {
	Name       string    // What's the name for this package (eg: launcher)
	Identifier string    // What is the identifier? (eg: kolide-app)
	Postinst   io.Reader // script to run after package install
	Prerm      io.Reader // script to run before package removal
	Root       string    // source directory to package
	SigningKey string    // key to sign packages with (platform specific behaviors)
	Version    string    // package version
}
