package log

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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

	logContext := l.extraKeyVals
	if bytes.Contains(p, []byte("Refusing to kill non-osqueryd process")) {
		logContext = append(logContext, getInfoAboutUnrecognizedProcessLockingPidfile(p)...)
	}

	msg := strings.TrimSpace(string(p))
	caller := extractOsqueryCaller(msg)
	if err := lf(l.logger).Log(append(logContext, "msg", msg, "caller", caller)...); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Occasionally, launcher will fail to start osquery -- in this case, osquery fails
// to lock the pidfile, and then will not kill the process using the pidfile because
// it does not appear to be another instance of osquery. We attempt to log additional
// information here about the process locking the pidfile.
// See: https://github.com/osquery/osquery/issues/7796
func getInfoAboutUnrecognizedProcessLockingPidfile(p []byte) []interface{} {
	pidStr := strings.TrimSpace(strings.TrimPrefix(string(p), "Refusing to kill non-osqueryd process"))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return []interface{}{"process_info_err", fmt.Sprintf("could not extract PID of non-osqueryd process %s using pidfile: %v", pidStr, err)}
	}

	unknownProcess, err := process.NewProcess(int32(pid))
	if err != nil {
		return []interface{}{"process_info_err", fmt.Sprintf("could not get non-osqueryd process %d using pidfile: %v", pid, err)}
	}

	// Gather as much info as we can about the process
	processInfo := make([]interface{}, 0)
	processInfo = append(processInfo, "process_name", getStringStat(unknownProcess.Name))
	processInfo = append(processInfo, "process_cmdline", getStringStat(unknownProcess.Cmdline))
	processInfo = append(processInfo, "process_status", getStringStat(unknownProcess.Status))
	processInfo = append(processInfo, "process_create_time", getIntStat(unknownProcess.CreateTime))
	processInfo = append(processInfo, "process_username", getStringStat(unknownProcess.Username))

	return processInfo
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
