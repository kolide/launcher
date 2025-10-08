//go:build !darwin
// +build !darwin

package testutil

// prepareBinaryForExecution is a no-op on non-Darwin platforms
func prepareBinaryForExecution(binaryPath string) error {
	return nil
}

// SignBinary is a no-op on non-Darwin platforms
func SignBinary(binaryPath string) error {
	return nil
}
