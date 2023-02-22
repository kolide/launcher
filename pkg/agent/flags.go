package agent

type flagInt interface {
	DebugServerData() bool
	ForceControlSubsystems() bool
}

var Flags flagInt = &initialFlagTest{}

type initialFlagTest struct{}

func (initialFlagTest) DebugServerData() bool {
	return false
}

// ForceControlSubsystems causes the control system to process each system. Regardless of the last hash value
func (initialFlagTest) ForceControlSubsystems() bool {
	return false
}
