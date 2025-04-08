//go:build !windows
// +build !windows

package osquerylogs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/shirou/gopsutil/v4/process"
)

// runAndLogPs runs ps filtering on the given PID, and logs the output.
func (l *OsqueryLogAdapter) runAndLogPs(pidStr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := allowedcmd.Ps(ctx, "-p", pidStr, "-o", "user,pid,ppid,pgid,stat,time,command")
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"error creating command to run ps on osqueryd pidfile",
			"err", err,
		)

		return
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"error running ps on non-osqueryd process using pidfile",
			"pid", pidStr,
			"err", err,
		)

		return
	}

	l.slogger.Log(context.TODO(), slog.LevelInfo,
		"ran ps on non-osqueryd process using pidfile",
		"pid", pidStr,
		"output", string(out),
	)
}

// runAndLogLsofByPID runs lsof filtering on the given PID, and logs the output.
func (l *OsqueryLogAdapter) runAndLogLsofByPID(pidStr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := allowedcmd.Lsof(ctx, "-R", "-n", "-p", pidStr)
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"error creating command to run lsof on osqueryd pidfile",
			"err", err,
		)

		return
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"error running lsof on non-osqueryd process using pidfile",
			"pid", pidStr,
			"err", err,
		)

		return
	}

	l.slogger.Log(context.TODO(), slog.LevelInfo,
		"ran lsof on non-osqueryd process using pidfile",
		"pid", pidStr,
		"output", string(out),
	)
}

// runAndLogLsofOnPidfile runs lsof filtering by the osquery pidfile, and logs
// the output.
func (l *OsqueryLogAdapter) runAndLogLsofOnPidfile() {
	fullPidfile := filepath.Join(l.rootDirectory, "osquery.pid")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := allowedcmd.Lsof(ctx, "-R", "-n", fullPidfile)
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"error creating command to run lsof on osqueryd pidfile",
			"err", err,
		)

		return
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"error running lsof on osqueryd pidfile",
			"pidfile", fullPidfile,
			"err", err,
		)

		return
	}

	l.slogger.Log(context.TODO(), slog.LevelInfo,
		"ran lsof on osqueryd pidfile",
		"pidfile", fullPidfile,
		"output", string(out),
	)
}

func getProcessesHoldingFile(ctx context.Context, pathToFile string) ([]*process.Process, error) {
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

	pidStrs := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(pidStrs) == 0 {
		return nil, errors.New("no processes found using file via lsof")
	}

	processes := make([]*process.Process, 0)
	for _, pidStr := range pidStrs {
		pid, err := strconv.ParseInt(pidStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid pid %s: %w", pidStr, err)
		}

		p, err := process.NewProcess(int32(pid))
		if err != nil {
			return nil, fmt.Errorf("getting process for %d: %w", pid, err)
		}
		processes = append(processes, p)
	}

	return processes, nil
}
