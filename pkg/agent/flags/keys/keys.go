package keys

// FlagKeys are named identifiers corresponding to flags
type FlagKey string

// When adding a new FlagKey:
// 1. Define the FlagKey identifier, and the string key value it corresponds to, in the block below
// 2. Add a getter and setter to the Flags interface (flags.go)
// 3. Implement the getter and setter in the Knapsack, which delegates the call to the FlagController
// 4. Implement the getter and setter in the FlagController, providing defaults, limits, and overrides
const (
	DesktopEnabled         FlagKey = "desktop_enabled_v1"
	DebugServerData        FlagKey = "debug_server_data"
	ForceControlSubsystems FlagKey = "force_control_subsystems"
	ControlServerURL       FlagKey = "control_server_url"
	ControlRequestInterval FlagKey = "control_request_interval"
	DisableControlTLS      FlagKey = "disable_control_tls"
	InsecureControlTLS     FlagKey = "insecure_control_tls"
)

func (key FlagKey) String() string {
	return string(key)
}

func ToFlagKeys(s []string) []FlagKey {
	f := make([]FlagKey, len(s))
	for i, v := range s {
		f[i] = FlagKey(v)
	}
	return f
}

// Returns the intersection of FlagKeys; keys which exist in both a and b.
func Intersection(a, b []FlagKey) []FlagKey {
	m := make(map[FlagKey]bool)
	var result []FlagKey

	for _, element := range a {
		m[element] = true
	}

	for _, element := range b {
		if m[element] {
			result = append(result, element)
		}
	}

	return result
}
