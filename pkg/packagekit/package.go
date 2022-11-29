package packagekit

// PackageOptions is the superset of all packaging options. Not all
// packages will support all options.
type PackageOptions struct {
	Identifier string // What is the identifier? (eg: kolide-app)
	Name       string // What's the name for this package (eg: launcher)
	Title      string // MacOS app bundle only -- the title displayed during installation
	Root       string // source directory to package
	Scripts    string // directory of packaging scripts (postinst, prerm, etc)
	Version    string // package version
	FlagFile   string // Path to the flagfile for configuration

	DisableService bool // Whether to install a system service in a disabled state

	AppleNotarizeAccountId   string   // The 10 character apple account id
	AppleNotarizeAppPassword string   // app password for notarization service
	AppleNotarizeUserId      string   // User id to authenticate to the notarization service with
	AppleSigningKey          string   // apple signing key
	WindowsSigntoolArgs      []string // Extra args for signtool. May be needed for finding a key
	WindowsUseSigntool       bool     // whether to use signtool.exe on windows

	WixPath        string // path to wix installation
	WixUI          bool   //include the wix ui or not
	WixSkipCleanup bool   // keep the temp dirs
}
