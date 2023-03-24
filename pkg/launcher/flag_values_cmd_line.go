package launcher

import (
	"github.com/kolide/launcher/pkg/agent/flags"
)

// CmdLineFlagValues converts command line options into the equivalent
// flagValues struct of FlagKeys and values
func CmdLineFlagValues(opts *Options) *flags.AnyFlagValues {
	f := flags.NewFlagValues[any]()
	// f.Set(DesktopEnabled, opts.DesktopEnabled)
	// f.Set(DebugServerData, opts.DebugServerData)
	// f.Set(ForceControlSubsystems, opts.ForceControlSubsystems)
	f.Set(flags.ControlServerURL, opts.ControlServerURL)
	f.Set(flags.ControlRequestInterval, opts.ControlRequestInterval)
	f.Set(flags.DisableControlTLS, opts.DisableControlTLS)
	f.Set(flags.InsecureControlTLS, opts.InsecureControlTLS)
	return f
}
