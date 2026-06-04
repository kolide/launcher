//go:build !windows

package permissions

// RestrictFileAccessToRootOnly is a no-op on non-Windows platforms because permissions
// are set on file creation instead.
func RestrictFileAccessToRootOnly(_ string) error {
	return nil
}
