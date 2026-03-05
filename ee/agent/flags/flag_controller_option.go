package flags

import "github.com/kolide/launcher/v2/pkg/launcher"

type Option func(*FlagController)

// WithStore sets the key/value store for control data
func WithCmdLineOpts(cmdLineOpts *launcher.Options) Option {
	return func(fc *FlagController) {
		fc.cmdLineOpts = cmdLineOpts
	}
}
