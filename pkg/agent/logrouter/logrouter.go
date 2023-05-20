// Package logrouter holds several connected tee loggers, log destinations, and a small amount of logic to route
// between them. It should allow for designs like:
//
//	-> systemLogger -----> tee ---->  filter 	--------> System Logs
//	                        |        by level
//	 	       	       	    |
//	 	       	       	    |
//	-> logger  -------------o------> tee ---------------> debug.json
//	                                  |
//	                                  +-----------------> memory ring
//	                                  |
//	                                  +-----> filter ---> persisted ring
//	                                         by level
//
// While several destinations are passed in, these fufill different purposes:
//  1. System Logs: This is meant for logs to be read be the end user. While end users can read the full debug.json,
//     system logs are meant to be more meaningful. (Note that the filter is applied upstream, and passed
//     a on initialization. This allows the command line flags to work)
//  2. debug.json: This is meant as a comprehensive log, suitable for sending to support
//  3. memory ring: This is meant to hold all recent logs. Because the debug level can be noisy
//  4. persisted ring: This handles recent info+ logs. It is persisted to the database, and can be queried

package logrouter

import (
	"errors"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	agenttypes "github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/ringlogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/kolide/launcher/pkg/persistentring"
)

type teeloggerInt interface {
	Add(loggers ...log.Logger)
	Log(keyvals ...interface{}) error
}

type storeInt interface {
	agenttypes.GetterSetterDeleterIterator
}

type ringLoggerInt interface {
	Log(keyvals ...interface{}) error
	GetAll() ([]map[string]any, error)
}

type logRouter struct {
	loggerTee   teeloggerInt
	systemTee   teeloggerInt
	logRing     ringLoggerInt // Holds the super set of logs, in memory
	persistRing ringLoggerInt // Only info and higher, persisted to a store
}

func New(systemLogger log.Logger) (*logRouter, error) {
	// We always have exactly one in memory ring logger. This recieves _all_ the logs. It's used to replay into new
	// desinatations, and is expected to be a query target for recent logs
	logRing, err := ringlogger.New(persistentring.NewInMemory(1000))
	if err != nil {
		return nil, fmt.Errorf("creating memory ringlogger: %w", err)
	}

	// Setup the internal logger, this rolls up all the internal destinations
	loggerTee := teelogger.New(logRing)

	// the systemLogger should copy anything it generates into the regular logger.
	systemTee := teelogger.New(systemLogger, loggerTee)

	return &logRouter{
		loggerTee: loggerTee,
		systemTee: systemTee,
		logRing:   logRing,
	}, nil
}

func (lr *logRouter) Log(keyvals ...interface{}) error {
	return lr.loggerTee.Log(keyvals...)
}

func (lr *logRouter) Logger() log.Logger {
	return lr.loggerTee
}

func (lr *logRouter) SystemLogger() log.Logger {
	return lr.systemTee
}

func (lr *logRouter) GetRecentLogs() ([]map[string]any, error) {
	if lr.persistRing == nil {
		return nil, nil
	}

	return lr.persistRing.GetAll()
}

func (lr *logRouter) GetRecentDebugLogs() ([]map[string]any, error) {
	return lr.logRing.GetAll()
}

// Replay will replay the logs from the memory ring to a new logger. Because we're serliazing everything down to a
// string, handling the leveling is cumbersome.
func (lr *logRouter) Replay(logger log.Logger) error {
	logs, err := lr.logRing.GetAll()
	if err != nil {
		return fmt.Errorf("gettings logs to replay: %w", err)
	}
	for _, l := range logs {
		var levelFn func(logger log.Logger) log.Logger

		pairs := make([]any, 2*len(l))
		pairs = append(pairs, "replayed", true)
		for k, v := range l {
			if k == level.Key() {
				switch v {
				case level.DebugValue().String():
					levelFn = level.Debug
				case level.InfoValue().String():
					levelFn = level.Info
				case level.WarnValue().String():
					levelFn = level.Warn
				case level.ErrorValue().String():
					levelFn = level.Error
				}
			}
			pairs = append(pairs, k, v)
		}
		if levelFn != nil {
			levelFn(logger).Log(pairs...)
		} else {
			logger.Log(pairs...)
		}
	}

	return nil
}

// AddPersistStore uses the provided store, to create a persisted ring. This is used to store important logs across
// restarts.
func (lr *logRouter) AddPersistStore(store storeInt) error {
	// Today, it only makes sense to have one of these. So error if not. (We could support an array, but _why_)
	if lr.persistRing != nil {
		return errors.New("already have a persited logger")
	}

	// Setup the logger
	r, err := persistentring.New(store, 1000)
	if err != nil {
		return fmt.Errorf("creating persistent ring: %w", err)
	}
	logger, err := ringlogger.New(r)
	if err != nil {
		return fmt.Errorf("creating stored ringlogger: %w", err)
	}

	lr.persistRing = logger

	// Most of this logger use is filtered.
	filteredLogger := level.NewFilter(logger, level.AllowInfo())

	// Replay logs. Note that there's a small race condition between replaying the logs, and adding the new logger.
	// However, it does not seem worth adding a mutext, and blocking all logging on it.
	if err := lr.Replay(filteredLogger); err != nil {
		return fmt.Errorf("setting up debug log: %w", err)
	}

	// Merge (but only info and higher)
	// Filter these to Info.
	lr.loggerTee.Add(filteredLogger)

	return nil
}

// AddDebugLog sets up a log destination to the provided path. It will be rotated.
func (lr *logRouter) AddDebugFile(logpath string) error {
	logger := locallogger.NewKitLogger(logpath)

	// Replay logs. Note that there's a small race condition between replaying the logs, and adding the new logger.
	// However, it does not seem worth adding a mutext, and blocking all logging on it.
	if err := lr.Replay(logger); err != nil {
		return fmt.Errorf("setting up debug log: %w", err)
	}

	// Merge
	lr.loggerTee.Add(logger)

	return nil
}
