//go:build !windows
// +build !windows

package osquerylogs

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
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
