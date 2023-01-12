package notify

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation -framework UserNotifications

#include <stdbool.h>
#include <stdlib.h>

bool sendNotification(char *title, char *content);
*/
import "C"
import (
	"fmt"
	"os"
	"strings"
	"unsafe"
)

func (d *DesktopNotifier) SendNotification(title, body string) error {
	// Check if we're running inside a bundle -- if we aren't, we should not attempt to send
	// a notification because it will cause a panic.
	if !isBundle() {
		return fmt.Errorf("cannot send notification because this application is not bundled")
	}

	titleCStr := C.CString(title)
	defer C.free(unsafe.Pointer(titleCStr))
	bodyCStr := C.CString(body)
	defer C.free(unsafe.Pointer(bodyCStr))

	success := C.sendNotification(titleCStr, bodyCStr)
	if !success {
		return fmt.Errorf("could not send notification %s", title)
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
