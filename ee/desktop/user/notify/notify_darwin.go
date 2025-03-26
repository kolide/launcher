//go:build darwin
// +build darwin

package notify

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation -framework UserNotifications -framework AppKit

#include <stdbool.h>
#include <stdlib.h>

bool sendNotification(char *cTitle, char *cBody, char *cActionUri);
void runNotificationListenerApp(void);
*/
import "C"
import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"unsafe"
)

type macNotifier struct {
	interrupt   chan struct{}
	interrupted atomic.Bool
}

func NewDesktopNotifier(_ *slog.Logger, _ string) *macNotifier {
	return &macNotifier{
		interrupt: make(chan struct{}),
	}
}

func (m *macNotifier) Execute() error {
	<-m.interrupt
	return nil
}

func (m *macNotifier) Interrupt(err error) {
	if m.interrupted.Load() {
		return
	}

	m.interrupted.Store(true)

	m.interrupt <- struct{}{}
}

func (m *macNotifier) Listen() {
	if isBundle() {
		C.runNotificationListenerApp()
	}
}

func (m *macNotifier) SendNotification(n Notification) error {
	// Check if we're running inside a bundle -- if we aren't, we should not attempt to send
	// a notification because it will cause a panic.
	if !isBundle() {
		return errors.New("cannot send notification because this application is not bundled")
	}

	titleCStr := C.CString(n.Title)
	defer C.free(unsafe.Pointer(titleCStr))
	bodyCStr := C.CString(n.Body)
	defer C.free(unsafe.Pointer(bodyCStr))
	actionUriCStr := C.CString(n.ActionUri)
	defer C.free(unsafe.Pointer(actionUriCStr))

	success := C.sendNotification(titleCStr, bodyCStr, actionUriCStr)
	if !success {
		return fmt.Errorf("could not send notification: %s", n.Title)
	}

	return nil
}

func isBundle() bool {
	currentExecutable, err := os.Executable()
	if err != nil {
		// Err on the safe side and say no, because launcher will shut down if we're wrong
		return false
	}

	return strings.Contains(currentExecutable, ".app")
}
