//go:build linux
// +build linux

package notify

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/godbus/dbus/v5"
	"os/exec"
	"time"
)

type dbusListener struct {
	logger    log.Logger
	conn      *dbus.Conn
	signal    chan *dbus.Signal
	interrupt chan struct{}
}

const (
	notificationServiceObj       = "/org/freedesktop/Notifications"
	notificationServiceInterface = "org.freedesktop.Notifications"
	signalActionInvoked          = "org.freedesktop.Notifications.ActionInvoked"
)

func newOsSpecificListener(logger log.Logger) (*dbusListener, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		level.Error(logger).Log("msg", "couldn't connect to dbus to start listener", "err", err)
		return nil, fmt.Errorf("could not connect to dbus: %w", err)
	}

	return &dbusListener{
		logger:    logger,
		conn:      conn,
		signal:    make(chan *dbus.Signal, 10),
		interrupt: make(chan struct{}),
	}, nil
}

func (d *dbusListener) Listen() error {
	if err := d.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(notificationServiceObj),
		dbus.WithMatchInterface(notificationServiceInterface),
	); err != nil {
		level.Error(d.logger).Log("msg", "couldn't add match signal", "err", err)
		return fmt.Errorf("couldn't register to listen to signals in dbus: %w", err)
	}
	d.conn.Signal(d.signal)

	for {
		select {
		case signal := <-d.signal:
			if signal == nil || signal.Name != signalActionInvoked {
				continue
			}

			// TODO: can we add additional matches or checks to make sure that this is a Kolide-originated notification?

			actionUri := signal.Body[1].(string)
			providers := []string{"/usr/bin/xdg-open", "/usr/bin/x-www-browser"}
			for _, provider := range providers {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				cmd := exec.CommandContext(ctx, provider, actionUri)
				cmd.Env = append(cmd.Env, "DISPLAY=:10.0")
				if err := cmd.Start(); err != nil {
					level.Error(d.logger).Log("msg", "couldn't start process", "err", err, "provider", provider)
				} else {
					break
				}
			}

		case <-d.interrupt:
			return nil
		}
	}
}

func (d *dbusListener) Interrupt(err error) {
	d.interrupt <- struct{}{}

	d.conn.RemoveSignal(d.signal)
	d.conn.RemoveMatchSignal(
		dbus.WithMatchObjectPath(notificationServiceObj),
		dbus.WithMatchInterface(notificationServiceInterface),
	)
}
