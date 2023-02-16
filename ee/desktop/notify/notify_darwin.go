package notify

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation -framework UserNotifications -framework AppKit

#include <stdbool.h>
#include <stdlib.h>

bool sendNotification(char *cTitle, char *cBody, char *cActionUri);
void runNotificationListenerApp(void);
void stopNotificationListenerApp(void);
*/
import "C"
import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"github.com/go-kit/kit/log"
)

type macNotifier struct {
	logger    log.Logger
	interrupt chan struct{}
}

func newOsSpecificNotifier(logger log.Logger, _ string) *macNotifier {
	return &macNotifier{
		logger:    logger,
		interrupt: make(chan struct{}),
	}
}

func (m *macNotifier) Listen() error {
	go func() {
		for {
			select {
			case <-m.interrupt:
				C.stopNotificationListenerApp()
				return
			}
		}
	}()

	C.runNotificationListenerApp()
	return nil
}

func (m *macNotifier) Interrupt(err error) {
	m.interrupt <- struct{}{}
}

func (m *macNotifier) SendNotification(title, body, actionUri string) error {
	// Check if we're running inside a bundle -- if we aren't, we should not attempt to send
	// a notification because it will cause a panic.
	if !isBundle() {
		return errors.New("cannot send notification because this application is not bundled")
	}

	titleCStr := C.CString(title)
	defer C.free(unsafe.Pointer(titleCStr))
	bodyCStr := C.CString(body)
	defer C.free(unsafe.Pointer(bodyCStr))
	actionUriCStr := C.CString(actionUri)
	defer C.free(unsafe.Pointer(actionUriCStr))

	success := C.sendNotification(titleCStr, bodyCStr, actionUriCStr)
	if !success {
		return fmt.Errorf("could not send notification: %s", title)
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
