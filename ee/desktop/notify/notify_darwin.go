package notify

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Foundation -framework UserNotifications

#include <stdbool.h>
#include <stdlib.h>

void sendNotificationWithFallback(char *title, char *content);
*/
import "C"
import (
	"fmt"
	"os/exec"
	"unsafe"

	"github.com/go-kit/kit/log/level"
)

func (n *Notifier) sendNotification(title, body string) {
	level.Debug(n.logger).Log("msg", "trying to send notification...", "title", title, "body", body)

	titleCStr := C.CString(title)
	defer C.free(unsafe.Pointer(titleCStr))
	bodyCStr := C.CString(body)
	defer C.free(unsafe.Pointer(bodyCStr))

	C.sendNotificationWithFallback(titleCStr, bodyCStr)
}

//export sendFallbackNotification
func sendFallbackNotification(titleCStr, bodyCStr *C.char) {
	title := C.GoString(titleCStr)
	body := C.GoString(bodyCStr)
	cmd := exec.Command("/usr/bin/osascript", "-e", fmt.Sprintf("display notification %q with title %q", body, title))
	cmd.Run()
}
