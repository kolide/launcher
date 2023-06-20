package logshipper

import (
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
)

const (
	truncatedFormatString = "%s[TRUNCATED]"
)

type logger struct {
	logger log.Logger
}

func newLogger(w io.Writer) *logger {
	return &logger{
		logger: log.NewJSONLogger(w),
	}
}

func (h *logger) Log(keyvals ...interface{}) error {
	filterResults(keyvals...)
	return h.logger.Log(keyvals...)
}

// filterResults filteres out the osquery results,
// which just make a lot of noise in our debug logs.
// It's a bit fragile, since it parses keyvals, but
// hopefully that's good enough
func filterResults(keyvals ...interface{}) {
	// Consider switching on `method` as well?
	for i := 0; i < len(keyvals); i += 2 {
		if keyvals[i] == "results" && len(keyvals) > i+1 {
			str, ok := keyvals[i+1].(string)
			if ok && len(str) > 100 {
				keyvals[i+1] = fmt.Sprintf(truncatedFormatString, str[0:99])
			}
		}
	}
}
