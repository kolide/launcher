package osquerylogs

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/process"
)

// OsqueryLogAdapater creates an io.Writer implementation useful for attaching
// to the osquery stdout/stderr
type OsqueryLogAdapter struct {
	slogger             *slog.Logger
	level               slog.Level
	rootDirectory       string
	lastLockfileLogTime time.Time
}

type Option func(*OsqueryLogAdapter)

func WithLevel(level slog.Level) Option {
	return func(l *OsqueryLogAdapter) {
		l.level = level
	}
}

var (
	callerRegexp  = regexp.MustCompile(`[\w.]+:\d+]`)
	pidRegex      = regexp.MustCompile(`Refusing to kill non-osqueryd process (\d+)`)
	lockfileRegex = regexp.MustCompile(`lock file: ([a-zA-Z0-9_\.\s\\\/\-:]*LOCK):`)
)

func extractOsqueryCaller(msg string) string {
	return strings.TrimSuffix(callerRegexp.FindString(msg), "]")
}

func NewOsqueryLogAdapter(slogger *slog.Logger, rootDirectory string, opts ...Option) *OsqueryLogAdapter {
	l := &OsqueryLogAdapter{
		slogger:       slogger,
		level:         slog.LevelInfo,
		rootDirectory: rootDirectory,
	}

	for _, opt := range opts {
		opt(l)
	}

	return l

}

func (l *OsqueryLogAdapter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("Accelerating distributed query checkins")) {
		// Skip writing this. But we still return len(p) so the caller thinks it was written
		return len(p), nil
	}

	// Occasionally, launcher will fail to start osquery -- in this case, osquery fails
	// to lock the pidfile, and then will not kill the process using the pidfile because
	// it does not appear to be another instance of osquery. We attempt to log additional
	// information here about the process locking the pidfile.
	// See: https://github.com/osquery/osquery/issues/7796
	if bytes.Contains(p, []byte("Refusing to kill non-osqueryd process")) {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"detected non-osqueryd process using pidfile, logging info about process",
		)
		gowrapper.Go(context.TODO(), l.slogger, func() {
			l.logInfoAboutUnrecognizedProcessLockingPidfile(p)
		})
	}

	// We have noticed the lock file occasionally locked when it shouldn't be -- we think this can happen
	// when osquery doesn't get adequate time to shut down gracefully. The held lockfile will prevent
	// osquery from starting up entirely.
	// See: https://github.com/kolide/launcher/issues/2004.
	if bytes.Contains(p, []byte("Rocksdb open failed")) {
		// We can get spammed with this log, but we don't want to do all the work to look up info about the process
		// using the lockfile each time we see this log -- make sure we only log once every 10 minutes at most.
		if time.Since(l.lastLockfileLogTime) > 10*time.Minute {
			l.lastLockfileLogTime = time.Now()
			l.slogger.Log(context.TODO(), slog.LevelError,
				"detected stale lockfile, logging info about file",
			)
			gowrapper.Go(context.TODO(), l.slogger, func() {
				l.logInfoAboutProcessHoldingLockfile(context.TODO(), p)
			})
		}
	}

	msg := strings.TrimSpace(string(p))
	caller := extractOsqueryCaller(msg)
	l.slogger.Log(context.TODO(), l.level, // nolint:sloglint // it's fine to not have a constant or literal here
		msg,
		"caller", caller,
	)

	return len(p), nil
}

// logInfoAboutUnrecognizedProcessLockingPidfile attempts to extract the PID of the process
// holding the osquery lock from the osquery log, and logs information about it if available.
func (l *OsqueryLogAdapter) logInfoAboutUnrecognizedProcessLockingPidfile(p []byte) {
	matches := pidRegex.FindAllStringSubmatch(string(p), -1)
	if len(matches) < 1 || len(matches[0]) < 2 {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"could not extract PID of non-osqueryd process using pidfile from log line",
			"log_line", string(p),
		)

		return
	}

	pidStr := strings.TrimSpace(matches[0][1]) // We want the group, not the full match
	pid, err := strconv.ParseInt(pidStr, 10, 32)
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"could not extract PID of non-osqueryd process using pidfile",
			"pid", pidStr,
			"err", err,
		)

		return
	}

	l.runAndLogPs(pidStr)
	l.runAndLogLsofByPID(pidStr)
	l.runAndLogLsofOnPidfile()

	unknownProcess, err := process.NewProcess(int32(pid))
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"could not get non-osqueryd process using pidfile",
			"pid", pid,
			"err", err,
		)

		return
	}

	// Gather as much info as we can about the process
	processInfo := []interface{}{"pid", pid}
	processInfo = append(processInfo, "name", getStringStat(unknownProcess.Name))
	processInfo = append(processInfo, "cmdline", getStringStat(unknownProcess.Cmdline))
	processInfo = append(processInfo, "status", getStringSliceStat(unknownProcess.Status))
	processInfo = append(processInfo, "create_time", getIntStat(unknownProcess.CreateTime))
	processInfo = append(processInfo, "username", getStringStat(unknownProcess.Username))
	processInfo = append(processInfo, "uids", getSliceStat(unknownProcess.Uids))

	// Add info about the parent, if available
	unknownProcessParent, _ := unknownProcess.Parent()
	if unknownProcessParent != nil {
		processInfo = append(processInfo, "parent_pid", unknownProcessParent.Pid)
		processInfo = append(processInfo, "parent_cmdline", getStringStat(unknownProcessParent.Cmdline))
		processInfo = append(processInfo, "parent_status", getStringSliceStat(unknownProcessParent.Status))
	}

	// Add system-level info
	processInfo = append(processInfo, "launcher_pid", os.Getpid())
	uptime, err := host.Uptime()
	if err == nil {
		processInfo = append(processInfo, "system_uptime", uptime)
	}

	l.slogger.Log(context.TODO(), slog.LevelInfo,
		"detected non-osqueryd process using pidfile",
		processInfo...,
	)
}

// getStringStat is a small wrapper around gopsutil/process functions
// to return the stat if available, or an error message if not, so
// that either way the info will be captured in the log.
func getStringStat(getFunc func() (string, error)) string {
	stat, err := getFunc()
	if err != nil {
		return fmt.Sprintf("could not get stat: %v", err)
	}
	return stat
}

// getStringSliceStat is a small wrapper around gopsutil/process functions
// to return the stat if available, or an error message if not, so
// that either way the info will be captured in the log.
func getStringSliceStat(getFunc func() ([]string, error)) string {
	stat, err := getFunc()
	if err != nil {
		return fmt.Sprintf("could not get stat: %v", err)
	}
	// We only use this function for `Status` at the moment, which is guaranteed to have one element when successful.
	return stat[0]
}

// getIntStat is a small wrapper around gopsutil/process functions
// to return the stat if available, or an error message if not, so
// that either way the info will be captured in the log.
func getIntStat(getFunc func() (int64, error)) string {
	stat, err := getFunc()
	if err != nil {
		return fmt.Sprintf("could not get stat: %v", err)
	}
	return fmt.Sprintf("%d", stat)
}

// getSliceStat is a small wrapper around gopsutil/process functions
// to return the stat if available, or an error message if not, so
// that either way the info will be captured in the log.
func getSliceStat(getFunc func() ([]uint32, error)) string {
	stat, err := getFunc()
	if err != nil {
		return fmt.Sprintf("could not get stat: %v", err)
	}
	return fmt.Sprintf("%+v", stat)
}

// logInfoAboutProcessHoldingLockfile logs information about the osquery database's lock file.
func (l *OsqueryLogAdapter) logInfoAboutProcessHoldingLockfile(ctx context.Context, p []byte) {
	executable, err := os.Executable()
	if err == nil && strings.Contains(executable, "__debug_bin") {
		return
	}

	matches := lockfileRegex.FindAllStringSubmatch(string(p), -1)
	if len(matches) < 1 || len(matches[0]) < 2 {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"could not extract lockfile path from log line",
			"log_line", string(p),
		)

		return
	}

	lockFilePath := strings.TrimSpace(matches[0][1]) // We want the group, not the full match
	if runtime.GOOS == "windows" {
		// for some reason the last separator in the path that we see from the logs is a forward slash even on windows.
		// this causes it to fail to match the open files we check later. so if we see that suffix on windows,
		// just flip that part of the path to correctly match the open file paths we'll check against
		lockSuffix := "/LOCK"
		if strings.HasSuffix(lockFilePath, lockSuffix) {
			lockFilePath = lockFilePath[:len(lockFilePath)-len(lockSuffix)] + "\\LOCK"
		}
	}

	infoToLog := []any{
		"lockfile_path", lockFilePath,
	}

	defer func() {
		l.slogger.Log(ctx, slog.LevelInfo,
			"detected stale osquery db lock file",
			infoToLog...,
		)
	}()

	// Check to see whether the process holding the file still exists
	processes, err := getProcessesHoldingFile(ctx, lockFilePath)
	if err != nil {
		infoToLog = append(infoToLog, "err", err)
		return
	}

	// Grab more info to log from the processes using the lockfile
	processStrs := make([]string, len(processes))
	for i, p := range processes {
		processStrs[i] = processStr(ctx, p)
	}
	infoToLog = append(infoToLog, "processes", processStrs)
}

func processStr(ctx context.Context, p *process.Process) string {
	name := "unknown"
	processOwner := "unknown"
	runningStatus := "unknown"
	cmdline := "unknown"

	if gotName, err := p.NameWithContext(ctx); err == nil {
		name = gotName
	}
	if gotUsername, err := p.UsernameWithContext(ctx); err == nil {
		processOwner = gotUsername
	}
	if gotIsRunning, err := p.IsRunningWithContext(ctx); err == nil {
		if gotIsRunning {
			runningStatus = "running"
		} else {
			runningStatus = "not running"
		}
	}
	if gotCmdline, err := p.CmdlineWithContext(ctx); err == nil {
		cmdline = gotCmdline
	}

	return fmt.Sprintf("process with name `%s` and PID %d belonging to user `%s` has current status `%s` (%s)", name, p.Pid, processOwner, runningStatus, cmdline)
}
