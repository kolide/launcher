// +build windows

package main

import (
	"os"
	"time"
	"fmt"
	"context"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/version"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/kolide/launcher/pkg/log/eventlog"
)

const serviceName = "launcher"

func main() {
	

	var logger log.Logger
	logger = logutil.NewCLILogger(true) //interactive



	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		logutil.Fatal(logger, "err", errors.Wrap(err, "cannot determine if session is interactive"))
		
	}

	run := debug.Run
	if !isIntSess {
		w,  err := eventlog.NewWriter(serviceName)
		if err != nil {
			logutil.Fatal(logger, "err", errors.Wrap(err, "create eventlog writer"))
		}
		defer w.Close()
		logger = eventlog.New(w)
		level.Debug(logger).Log("msg", "daemonized service start requested")
		run = svc.Run
	}

	if isSubCommand() {
		switch strings.ToLower(os.Args[1]) {
		case "install":
			err = installService(serviceName, "Kolide Osquery Launcher")
		case "remove":
			err = removeService(serviceName)
		case "start":
			err = startService(serviceName)
		case "stop":
			err = controlService(serviceName, svc.Stop, svc.Stopped)	
		}
		if err != nil {
			logutil.Fatal(logger, "err", errors.Wrap(err, "run"))
		}
		return
	}

	opts, err := parseOptions()
	if err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	// handle --version
	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	// handle --usage
	if opts.developerUsage {
		developerUsage()
		os.Exit(0)
	}

	err = run(serviceName, &winSvc{logger: logger, opts: opts})
	if err != nil {
		logutil.Fatal(logger, "err", errors.Wrap(err, "run"))
	}

}



type winSvc struct{
	logger log.Logger
	opts *options
}

func (w *winSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	level.Debug(w.logger).Log("msg", "windows service starting")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err := runLauncher(ctx, cancel, w.opts, w.logger)
		if err != nil {
			level.Info(w.logger).Log("err", err)
			changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
			os.Exit(1)
		}
	}()

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
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

func exePath() (string, error) {
	prog := os.Args[0]
	p, err := filepath.Abs(prog)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(p)
	if err == nil {
		if !fi.Mode().IsDir() {
			return p, nil
		}
		err = fmt.Errorf("%s is directory", p)
	}
	if filepath.Ext(p) == "" {
		p += ".exe"
		fi, err := os.Stat(p)
		if err == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}
			err = fmt.Errorf("%s is directory", p)
		}
	}
	return "", err
}

func isSubCommand() bool {
	if len(os.Args) > 2 {
		return false
	}

	subCommands := []string{
		"install",
		"remove",
		"start",
		"stop",
	}

	for _, sc := range subCommands {
		if sc == os.Args[1] {
			return true
		}
	}

	return false
}

func installService(name, desc string) error {
	exepath, err := exePath()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", name)
	}
	s, err = m.CreateService(name, exepath, mgr.Config{DisplayName: desc}, "is", "auto-started")
	if err != nil {
		return err
	}
	defer s.Close()
	return nil
}

func removeService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("service %s is not installed", name)
	}
	defer s.Close()
	err = s.Delete()
	return err
}

func startService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func controlService(name string, c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}