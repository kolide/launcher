package main

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

// setupLoggers creates the various loggers that launcher uses. We have two input loggers, a "systemLogger" that sends
// things to the system log facility, and an internal logger. These route to various places based on level.
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
//     system logs are meant to be more meaningful.
//  2. debug.json
//  3. memory ring
//  4. persisted ring
func setupLoggers(systemLogger log.Logger, rootDirectory string, store storeInt) (log.Logger, log.Logger, error) {
	destinations := make([]log.Logger, 3)

	// If we have a root directory, we should setup locallogging to the debug.json log.
	if rootDirectory != "" {
		destinations = append(destinations, locallogger.NewKitLogger(filepath.Join(rootDirectory, "debug.json")))
	}

	// We always have an in memory ring logger
	mrl, err := ringlogger.New(persistentring.NewInMemory(1000))
	if err != nil {
		return nil, nil, fmt.Errorf("creating memory ringlogger: %w", err)
	}
	destinations = append(destinations, mrl)

	// If we have a store, we should log recent important logs. We filter what goes into
	// the store, since debug is noisy enough to cause extra transaction load.
	if store != nil {
		r, err := persistentring.New(store, 1000)
		if err != nil {
			return nil, nil, fmt.Errorf("creating persistent ring: %w", err)
		}
		rl, err := ringlogger.New(r)
		if err != nil {
			return nil, nil, fmt.Errorf("creating stored ringlogger: %w", err)
		}

		destinations = append(destinations, level.NewFilter(rl, level.AllowInfo()))
	}

	// Setup the internal logger, this rolls up all the internal destinations
	logger := teelogger.New(destinations...)

	// the systemLogger should copy anything it generates into the regular logger.
	systemLogger = teelogger.New(systemLogger, logger)

	return logger, systemLogger, nil
}
