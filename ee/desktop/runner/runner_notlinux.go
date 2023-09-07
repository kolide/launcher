//go:build !linux
// +build !linux

package runner

import "context"

func (r *DesktopUsersProcessesRunner) userEnvVars(_ context.Context, _ string) map[string]string {
	return nil
}
