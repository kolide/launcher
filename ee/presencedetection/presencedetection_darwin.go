//go:build darwin
// +build darwin

package presencedetection

/*
#cgo CFLAGS: -x objective-c -fmodules -fblocks
#cgo LDFLAGS: -framework CoreFoundation -framework LocalAuthentication -framework Foundation
#include <stdlib.h>
#include "auth.h"
*/
import "C"
import (
	"fmt"
	"time"
	"unsafe"
)

func Detect(reason string, timeout time.Duration) (bool, error) {
	reasonStr := C.CString(reason)
	defer C.free(unsafe.Pointer(reasonStr))

	// Convert timeout to nanoseconds
	timeoutNs := C.int64_t(timeout.Nanoseconds())

	result := C.Authenticate(reasonStr, timeoutNs)

	if result.error_msg != nil {
		defer C.free(unsafe.Pointer(result.error_msg))
	}
	errorMessage := C.GoString(result.error_msg)

	if result.success {
		return true, nil
	}

	return false, fmt.Errorf("authentication failed: %d %s", int(result.error_code), errorMessage)
}
