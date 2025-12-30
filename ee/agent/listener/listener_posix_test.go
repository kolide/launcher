//go:build darwin || linux

package listener

// hasPermissionsToRunTest always return true for non-windows platforms since
// elveated permissions are not required to run the tests
func hasPermissionsToRunTest() bool {
	return true
}
