//go:build !linux
// +build !linux

package tuf

func patchExecutable(executableLocation string) error {
	return nil
}
