package log

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/process"
)

// OsqueryLogAdapater creates an io.Writer implementation useful for attaching
// to the osquery stdout/stderr
type OsqueryLogAdapter struct {
	logger       kitlog.Logger
	levelFunc    func(kitlog.Logger) kitlog.Logger
	extraKeyVals []interface{} // log.With expects an interface, not string
}

type Option func(*OsqueryLogAdapter)

func WithKeyValue(key, value string) Option {
	return func(l *OsqueryLogAdapter) {
		l.extraKeyVals = append(l.extraKeyVals, key, value)
	}
}

func WithLevelFunc(lf func(kitlog.Logger) kitlog.Logger) Option {
	return func(l *OsqueryLogAdapter) {
		l.levelFunc = lf
	}
}

var callerRegexp = regexp.MustCompile(`[\w.]+:\d+]`)

var pidRegex = regexp.MustCompile(`Refusing to kill non-osqueryd process (\d+)`)

func extractOsqueryCaller(msg string) string {
	return strings.TrimSuffix(callerRegexp.FindString(msg), "]")
}

func NewOsqueryLogAdapter(logger kitlog.Logger, opts ...Option) *OsqueryLogAdapter {
	l := &OsqueryLogAdapter{
		logger:       logger,
		levelFunc:    level.Debug,
		extraKeyVals: []interface{}{},
	}

	for _, opt := range opts {
		opt(l)
	}

	return l

}

func (l *OsqueryLogAdapter) Write(p []byte) (int, error) {
	// Work around osquery being overly verbose with it's logs
	// see: https://github.com/osquery/osquery/pull/6271
	lf := l.levelFunc
	if bytes.Contains(p, []byte("Executing scheduled query pack")) {
		lf = level.Debug
	}

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
		l.logInfoAboutUnrecognizedProcessLockingPidfile(p)
	}

	msg := strings.TrimSpace(string(p))
	caller := extractOsqueryCaller(msg)
	if err := lf(l.logger).Log(append(l.extraKeyVals, "msg", msg, "caller", caller)...); err != nil {
		return 0, err
	}
	return len(p), nil
}

// logInfoAboutUnrecognizedProcessLockingPidfile attempts to extract the PID of the process
// holding the osquery lock from the osquery log, and logs information about it if available.
func (l *OsqueryLogAdapter) logInfoAboutUnrecognizedProcessLockingPidfile(p []byte) {
	matches := pidRegex.FindAllStringSubmatch(string(p), -1)
	if len(matches) < 1 || len(matches[0]) < 2 {
		level.Debug(l.logger).Log(
			"msg", "could not extract PID of non-osqueryd process using pidfile from log line",
			"log_line", string(p),
		)
		return
	}

	pidStr := strings.TrimSpace(matches[0][1]) // We want the group, not the full match
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		level.Debug(l.logger).Log(
			"msg", "could not extract PID of non-osqueryd process using pidfile",
			"pid", pidStr,
			"err", err,
		)
		return
	}

	unknownProcess, err := process.NewProcess(int32(pid))
	if err != nil {
		level.Debug(l.logger).Log(
			"msg", "could not get non-osqueryd process using pidfile",
			"pid", pid,
			"err", err,
		)
		return
	}

	// Gather as much info as we can about the process
	processInfo := []interface{}{"pid", pid}
	processInfo = append(processInfo, "name", getStringStat(unknownProcess.Name))
	processInfo = append(processInfo, "cmdline", getStringStat(unknownProcess.Cmdline))
	processInfo = append(processInfo, "status", getStringStat(unknownProcess.Status))
	processInfo = append(processInfo, "create_time", getIntStat(unknownProcess.CreateTime))
	processInfo = append(processInfo, "username", getStringStat(unknownProcess.Username))
	processInfo = append(processInfo, "uids", getSliceStat(unknownProcess.Uids))

	// Add info about the parent, if available
	unknownProcessParent, _ := unknownProcess.Parent()
	if unknownProcessParent != nil {
		processInfo = append(processInfo, "parent_pid", unknownProcessParent.Pid)
		processInfo = append(processInfo, "parent_cmdline", getStringStat(unknownProcessParent.Cmdline))
		processInfo = append(processInfo, "parent_status", getStringStat(unknownProcessParent.Status))
	}

	// Add system-level info
	processInfo = append(processInfo, "launcher_pid", os.Getpid())
	uptime, err := host.Uptime()
	if err == nil {
		processInfo = append(processInfo, "system_uptime", uptime)
	}

	level.Debug(l.logger).Log(append(processInfo, "msg", "detected non-osqueryd process using pidfile")...)
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
func getSliceStat(getFunc func() ([]int32, error)) string {
	stat, err := getFunc()
	if err != nil {
		return fmt.Sprintf("could not get stat: %v", err)
	}
	return fmt.Sprintf("%+v", stat)
}
