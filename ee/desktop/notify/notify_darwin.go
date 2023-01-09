package notify

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation -framework UserNotifications

#include <stdbool.h>
#include <stdlib.h>

void sendNotification(char *title, char *content);
*/
import "C"
import (
	"fmt"
	"os"
	"strings"
	"unsafe"
)

func (n *Notifier) sendNotification(title, body string) error {
	if !isBundle() {
		return fmt.Errorf("cannot send notification because this application is not bundled")
	}

	titleCStr := C.CString(title)
	defer C.free(unsafe.Pointer(titleCStr))
	bodyCStr := C.CString(body)
	defer C.free(unsafe.Pointer(bodyCStr))

	C.sendNotification(titleCStr, bodyCStr)

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
