//go:build windows
// +build windows

package eventlog

import (
	"bytes"
	"io"
	"sync"
	"reflect"

	"github.com/go-kit/kit/log"
)

// New creates a log.Logger that writes to the Windows Event Log.
// The Logger formats event data using logfmt.
func New(w *Writer) log.Logger {
	l := &eventLogger{
		w:         w,
		newLogger: log.NewLogfmtLogger,
		bufPool: sync.Pool{New: func() interface{} {
			return &loggerBuf{}
		}},
	}
	return l
}

type eventLogger struct {
	w         *Writer
	bufPool   sync.Pool
	newLogger func(io.Writer) log.Logger
}

func (l *eventLogger) Log(keyvals ...interface{}) error {
	lb := l.getLoggerBuf()
	defer l.putLoggerBuf(lb)

	// fmtlogger does not support array, chan, func, slice, struct, or map
	// so we'll do any pre-processing for these types here
	formattedKeyVals := make([]interface{}, len(keyvals))
	for idx, val := range keyvals {
		switch knownValue := val.(type) {
		case reflect.Array, reflect.Chan, reflect.Func, reflect.Map, reflect.Slice, reflect.Struct:
			formattedKeyVals[idx] = fmt.Sprintf("%+v", val)
		default:
			formattedKeyVals[idx] = val
	}

	if err := lb.logger.Log(formattedKeyVals...); err != nil {
		return err
	}

	_, err := l.w.Write(lb.buf.Bytes())
	return err
}

type loggerBuf struct {
	buf    *bytes.Buffer
	logger log.Logger
}

func (l *eventLogger) getLoggerBuf() *loggerBuf {
	lb := l.bufPool.Get().(*loggerBuf)
	if lb.buf == nil {
		lb.buf = &bytes.Buffer{}
		lb.logger = log.With(l.newLogger(lb.buf), "caller", log.Caller(4))
	} else {
		lb.buf.Reset()
	}
	return lb
}

func (l *eventLogger) putLoggerBuf(lb *loggerBuf) {
	l.bufPool.Put(lb)
}
