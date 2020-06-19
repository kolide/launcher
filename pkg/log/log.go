package log

import (
	"bytes"
	"regexp"
	"strings"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
		lf = level.Debug
	}

	// TODO: This is a short term hack.  In
	// https://github.com/osquery/osquery/pull/6271 osquery
	// shifted some debugging info from INFO to VERBOSE. This has
	// the unfortunate effect of making it hard to correlate
	// distributed query logs with the distributed query that
	// caused them. While we're thinking through the longer term
	// fix, we have a quick mitagation in dropping osquery into
	// verbose mode, but we also want to filter out the
	// unimportant logs
	if bytes.HasPrefix(p, []byte("I")) && !bytes.Contains(p, []byte("Executing scheduled query")) {
		lf = level.Debug
	}

	msg := strings.TrimSpace(string(p))
	caller := extractOsqueryCaller(msg)
	if err := lf(l.logger).Log(append(l.extraKeyVals, "msg", msg, "caller", caller)...); err != nil {
		return 0, err
	}
	return len(p), nil
}
