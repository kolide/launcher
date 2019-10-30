package log

import (
	"regexp"
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// OsqueryLogAdapater creates an io.Writer implementation useful for attaching
// to the osquery stdout/stderr
type OsqueryLogAdapter struct {
	kitlog.Logger
}

var callerRegexp = regexp.MustCompile(`[\w.]+:\d+]`)

func extractOsqueryCaller(msg string) string {
	return strings.TrimSuffix(callerRegexp.FindString(msg), "]")
}

func (l *OsqueryLogAdapter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	caller := extractOsqueryCaller(msg)
	if err := level.Debug(l.Logger).Log("msg", msg, "caller", caller); err != nil {
		return 0, err
	}
	return len(p), nil
}
