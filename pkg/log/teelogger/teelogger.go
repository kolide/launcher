// Package filetee provides a go-kit compatible logger, which will
// mirror logs into a tmp file. This is designed to aid in debugging.
package teelogger

import (
	"github.com/go-kit/kit/log"
)

type teeLogger struct {
	loggers []log.Logger
}

func New(loggers ...log.Logger) log.Logger {
	l := &teeLogger{
		loggers: loggers,
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
