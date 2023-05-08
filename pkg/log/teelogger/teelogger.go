// Package filetee provides a go-kit compatible log mirroring tool.
package teelogger

import (
	"github.com/go-kit/kit/log"
)

type teeLogger struct {
	loggers []log.Logger
}

func New(loggers ...log.Logger) log.Logger {

	l := &teeLogger{
		loggers: make([]log.Logger, 0),
	}
	for _, logger := range loggers {
		if logger == nil {
			continue
		}
		l.loggers = append(l.loggers, logger)
	}
	return l
}

// Log will log to each logger. If any of them error, it will return a
// random error.
func (l *teeLogger) Log(keyvals ...interface{}) error {
	var randomErr error
	for _, logger := range l.loggers {
		if err := logger.Log(keyvals...); err != nil {
			randomErr = err
		}
	}

	return randomErr
}
