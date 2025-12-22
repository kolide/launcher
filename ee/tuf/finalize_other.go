//go:build !linux

package tuf

func patchExecutable(_ string) error {
	return nil
}
