// +build windows

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/log/eventlog"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/mgr"
)

func runWindowsSvc(args []string) error {
	eventLogWriter, err := eventlog.NewWriter(serviceName)
	if err != nil {
		return errors.Wrap(err, "create eventlog writer")
	}
	defer eventLogWriter.Close()

	logger := eventlog.New(eventLogWriter)
	level.Debug(logger).Log("msg", "service start requested")

	run := svc.Run
	return run(serviceName, &winSvc{logger: logger, args: args})
}

func runWindowsSvcForeground(args []string) error {
	// Foreground mode is inherently a debug mode. So we start the
	// logger in debugging mode, instead of looking at opts.debug
	logger := logutil.NewCLILogger(true)
	level.Debug(logger).Log("msg", "foreground service start requested (debug mode)")

	run := debug.Run
	return run(serviceName, &winSvc{logger: logger, args: args})
}

type winSvc struct {
	logger log.Logger
	args   []string
}

func (w *winSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	level.Debug(w.logger).Log("msg", "windows service starting")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO: needs a gofunc probably
	if err := fixRecoveryActions(serviceName); err != nil {
		level.Info(w.logger).Log("msg", "Failed to fixRecoveryActions", "err", err)
		// TODO: Do we actually want to exit here? Not sure.
		changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
		os.Exit(1)
	}

	go func() {
		err := runLauncher(ctx, cancel, w.args, w.logger)
		if err != nil {
			level.Info(w.logger).Log("err", err, "stack", fmt.Sprintf("%+v", err))
			changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
			os.Exit(1)
		}

		level.Info(w.logger).Log("msg", "runLauncher gofunc ended cleanly", "pid", os.Getpid())

		// Case 1 -- Nothing here
		//
		// If we do not exit here, we sorta just hang. This doesn't seem
		// surprising -- What else would happen. The launcher go routine
		// ends, but the signal handling forloop remains.

		// Case 2 -- os.Exit(0)
		//
		// Logs stop, and the service shows as stopped. Eg: windows
		// services saw the clean exit and assumed it was
		// intentional. Note that this may depend on some service
		// installation parameter.
		level.Info(w.logger).Log("msg", "exit(0)")
		os.Exit(0)

		// Case 3 -- os.Exit(1)
		//
		// Same as Case 2. Makes me think something is set oddly in the
		// windows service recovery stuff. It really oughgt be
		// retrying. Need to figure out how to get to those settings
		//level.Info(w.logger).Log("msg", "let's exit(1)")
		//os.Exit(1)
	}()

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				level.Info(w.logger).Log("case", "Interrogate")
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				level.Info(w.logger).Log("case", "stop/shutdown")
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				time.Sleep(100 * time.Millisecond)
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
				return
			default:
				level.Info(w.logger).Log("err", "unexpected control request", "control_request", c)
			}
		}
	}
}

// Fix the Recovery Actions.
//
// This is all a hack around MSI's inability to set the recovery actions.
//
// Doing this requires the service name. We ought be able to get it
// inside the service, but I can't see how. So, we'll make some
// assumptions about how the service has been installed. Bummer.
func fixRecoveryActions(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return errors.Wrap(err, "mgr.Connect")
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return errors.Errorf("service %s is not installed", name)
	}
	defer s.Close()

	// Action, and time to wait before performing action
	ra := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 1 * time.Second}

	// How many seconds of stable daemon activity resets the RecoveryAction cycle
	resetPeriod := uint32(3)

	if err := s.SetRecoveryActions([]mgr.RecoveryAction{ra}, resetPeriod); err != nil {
		return errors.Wrap(err, "SetRecoveryActions")
	}

	return nil
}
