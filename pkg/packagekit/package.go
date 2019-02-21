package packagekit

// PackageOptions is the superset of all packaging options. Not all
// packages will support all options.
type PackageOptions struct {
	Identifier string // What is the identifier? (eg: kolide-app)
	Name       string // What's the name for this package (eg: launcher)
	Root       string // source directory to package
	Scripts    string // directory of packaging scripts (postinst, prerm, etc)
	SigningKey string // key to sign packages with (platform specific behaviors)
	Version    string // package version
	FlagFile   string // Path to the flagfile for configuration
}
