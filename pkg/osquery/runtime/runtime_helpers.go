//go:build !windows
// +build !windows

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/shirou/gopsutil/v3/process"
)

func setpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// kill process group kills a process and all its children.
func killProcessGroup(cmd *exec.Cmd) error {
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill process group %d: %w", cmd.Process.Pid, err)
	}
	return nil
}

func SocketPath(rootDir string, id string) string {
	return filepath.Join(rootDir, fmt.Sprintf("osquery-%s.sock", id))
}

func platformArgs() []string {
	return nil
}

func isExitOk(_ error) bool {
	return false
}

func getProcessHoldingFile(ctx context.Context, pathToFile string) (*process.Process, error) {
	cmd, err := allowedcmd.Lsof(ctx, "-t", pathToFile)
	if err != nil {
		return nil, fmt.Errorf("creating lsof command: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running lsof: %w", err)
	}

	pidStr := strings.TrimSpace(string(out))
	if pidStr == "" {
		return nil, errors.New("no process found using file via lsof")
	}

	pid, err := strconv.ParseInt(pidStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid pid %s: %w", pidStr, err)
	}

	return process.NewProcess(int32(pid))
}
