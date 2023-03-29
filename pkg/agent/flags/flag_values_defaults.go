package flags

import (
	"time"
)

// DefaultFlagValues returns a FlagValues struct of default FlagKeys and values
func DefaultFlagValues() *FlagValues {
	f := NewFlagValues()

	// Below is a list of every FlagKey and it's default value.
	// When adding a new FlagKey, make sure to add it's default value below.
	f.Set(DesktopEnabled, false)
	f.Set(DebugServerData, false)
	f.Set(ForceControlSubsystems, false)
	f.Set(ControlServerURL, "")
	f.Set(ControlRequestInterval, int64(60*time.Second))
	f.Set(DisableControlTLS, false)
	f.Set(InsecureControlTLS, false)
	return f
}
