// Package logthicket host the tangled loggers, destinations, and filters we use in the agent. It encodes
// routing logic between them. It is isolated from knapsack, because we want to create it before knapsack, and use it
// for the early logging. It should be passed into knapsack, and not used outside main.
//
// We have two inputs:
// 1. a "systemLogger" that sends things to the system log facility
// 2. an internal logger, which routes to internal destinations
//
// These are additionally routed by level:
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
// These destinations serve somewhat specialized purposes:
//  1. System Logs: This is meant for logs to be read be the end user. While end users can read the full debug.json,
//     system logs are meant to be more meaningful. (Note that the filter is applied upstream, and passed
//     a on initialization. This allows the command line flags to work)
//  2. debug.json: This is meant as a comprehensive log, suitable for sending to support
//  3. memory ring: This is meant to hold all recent logs. Because the debug level can be noisy
//  4. persisted ring: This handles recent info+ logs. It is persisted to the database, and can be queried
package logthicket

import (
	"fmt"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	agenttypes "github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/ringlogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/kolide/launcher/pkg/persistentring"
)

type storeInt interface {
	agenttypes.GetterSetterDeleterIterator
}

type ringLogger interface {
	Log(keyvals ...interface{}) error
	GetAll() ([]map[string]any, error)
}

type logThicket struct {
	systemLogger log.Logger
	logger       log.Logger
	persistRing  ringLogger
	debugRing    ringLogger
}

// New returns an agent log thicket. It takes in a system logger, as opposed to creating one, so that the debug
// filtering and platform specific loggers are in main. Perhaps that will change.
func New(systemLogger log.Logger, rootDirectory string, store storeInt) (*logThicket, error) {
	destinations := make([]log.Logger, 0, 3)

	// If we have a root directory, we should setup local logging to the debug.json log.
	if rootDirectory != "" {
		destinations = append(destinations, locallogger.NewKitLogger(filepath.Join(rootDirectory, "debug.json")))
	}

	// We always have an in memory ring logger
	mrl, err := ringlogger.New(persistentring.NewInMemory(1000))
	if err != nil {
		return nil, fmt.Errorf("creating memory ringlogger: %w", err)
	}
	destinations = append(destinations, mrl)

	r, err := persistentring.New(store, 1000)
	if err != nil {
		return nil, fmt.Errorf("creating persistent ring: %w", err)
	}
	rl, err := ringlogger.New(r)
	if err != nil {
		return nil, fmt.Errorf("creating stored ringlogger: %w", err)
	}

	destinations = append(destinations, level.NewFilter(rl, level.AllowInfo()))

	// Setup the internal logger, this rolls up all the internal destinations
	logger := teelogger.New(destinations...)

	// the systemLogger should copy anything it generates into the regular logger.
	systemLogger = teelogger.New(systemLogger, logger)

	return &logThicket{
		systemLogger: systemLogger,
		logger:       logger,
		persistRing:  rl,
		debugRing:    mrl,
	}, nil
}

func (lt *logThicket) Logger() log.Logger {
	return lt.logger
}

func (lt *logThicket) SystemLogger() log.Logger {
	return lt.systemLogger
}

func (lt *logThicket) GetRecentLogs() ([]map[string]any, error) {
	return lt.persistRing.GetAll()
}

func (lt *logThicket) GetRecentDebugLogs() ([]map[string]any, error) {
	return lt.debugRing.GetAll()
}
