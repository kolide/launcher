// +build !windows

package eventlog

type Writer struct {
}

func NewWriter(name string) (*Writer, error) {
	panic("windows only")
}
