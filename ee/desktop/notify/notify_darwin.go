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
	"unsafe"
)

func (n *Notifier) sendNotification(title, body string) {
	titleCStr := C.CString(title)
	defer C.free(unsafe.Pointer(titleCStr))
	bodyCStr := C.CString(body)
	defer C.free(unsafe.Pointer(bodyCStr))

	C.sendNotification(titleCStr, bodyCStr)
}
