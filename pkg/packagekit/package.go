package packagekit

// PackageOptions is the superset of all packaging options. Not all
// packages will support all options.
type PackageOptions struct {
	Identifier string // What is the identifier? (eg: kolide-app)
	Name       string // What's the name for this package (eg: launcher)
	Root       string // source directory to package
	Scripts    string // directory of packaging scripts (postinst, prerm, etc)
	Version    string // package version
	FlagFile   string // Path to the flagfile for configuration

	DisableService bool // Whether to install a system service in a disabled state

	AppleSigningKey     string   // apple signing key
	WindowsUseSigntool  bool     // whether to use signtool.exe on windows
	WindowsSigntoolArgs []string // Extra args for signtool. May be needed for finding a key

	WixPath        string // path to wix installation
	WixUI          bool   //include the wix ui or not
	WixSkipCleanup bool   // keep the temp dirs
}
