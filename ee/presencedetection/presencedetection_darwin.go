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
	"unsafe"
)

func Detect(reason string) (bool, error) {
	reasonStr := C.CString(reason)
	defer C.free(unsafe.Pointer(reasonStr))

	result := C.Authenticate(reasonStr)

	// Convert C error message to Go string
	if result.error_msg != nil {
		defer C.free(unsafe.Pointer(result.error_msg))
	}
	errorMessage := C.GoString(result.error_msg)

	// Return success or failure, with an error if applicable
	if result.success {
		return true, nil
	}

	return false, fmt.Errorf("authentication failed: %d %s", int(result.error_code), errorMessage)
}
