package agent

type flagInt interface {
	DebugServerData() bool
}

var Flags flagInt = &initialFlagTest{}

type initialFlagTest struct{}

func (initialFlagTest) DebugServerData() bool {
	return true
}
