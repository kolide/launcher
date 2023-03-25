package launcher

import (
	"github.com/kolide/launcher/pkg/agent/flags"
)

// CmdLineFlagValues converts command line options into the equivalent
// flagValues struct of FlagKeys and values
func CmdLineFlagValues(opts *Options) *flags.AnyFlagValues {
	f := flags.NewFlagValues[any]()

	// Not every FlagKey has (or needs) an associated cmd line option.
	// For those that do, make sure to add it below.
	f.Set(flags.ControlServerURL, opts.ControlServerURL)
	f.Set(flags.ControlRequestInterval, opts.ControlRequestInterval)
	f.Set(flags.DisableControlTLS, opts.DisableControlTLS)
	f.Set(flags.InsecureControlTLS, opts.InsecureControlTLS)
	return f
}
