package flags

import (
	"time"
)

// DefaultFlagValues returns a flagValues struct of default FlagKeys and values
func DefaultFlagValues() *AnyFlagValues {
	f := NewFlagValues[any]()
	f.Set(DesktopEnabled, false)
	f.Set(DebugServerData, false)
	f.Set(ForceControlSubsystems, false)
	f.Set(ControlServerURL, "")
	f.Set(ControlRequestInterval, 60*time.Second)
	f.Set(DisableControlTLS, false)
	f.Set(InsecureControlTLS, false)
	return f
}
