package table

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#include <CoreFoundation/CoreFoundation.h>
*/
import "C"
import (
	"fmt"
	"reflect"
	"unsafe"
)

func copyPreferenceValue(key, domain, username string) interface{} {
	keyCFString := CFStringRef(key)
	defer C.CFRelease((C.CFTypeRef)(keyCFString))
	domainCFString := CFStringRef(domain)
	defer C.CFRelease((C.CFTypeRef)(domainCFString))
	usernameCFString := CFStringRef(username)
	defer C.CFRelease((C.CFTypeRef)(usernameCFString))

	val := C.CFPreferencesCopyValue(
		keyCFString, domainCFString, usernameCFString, C.kCFPreferencesAnyHost,
	)
	defer C.CFRelease((C.CFTypeRef)(val))
	return goValueFromCFPlistRef(val)
}

// CFStringRef returns a C.CFStringRef which must be released with C.CFRelease
func CFStringRef(s string) C.CFStringRef {
	return C.CFStringCreateWithCString(C.kCFAllocatorDefault, C.CString(s), C.kCFStringEncodingUTF8)
}

func GoBoolean(ref C.CFBooleanRef) bool {
	return ref == C.kCFBooleanTrue
}

func GoInt(ref C.CFNumberRef) int {
	var n int
	C.CFNumberGetValue(ref, C.CFNumberGetType(ref), unsafe.Pointer(&n))
	return n
}

func GoString(ref C.CFStringRef) string {
	length := C.CFStringGetLength(ref)
	if length == 0 {
		// empty string
		return ""
	}
	cfRange := C.CFRange{0, length}
	enc := C.CFStringEncoding(C.kCFStringEncodingUTF8)
	var usedBufLen C.CFIndex
	if C.CFStringGetBytes(ref, cfRange, enc, 0, C.false, nil, 0, &usedBufLen) > 0 {
		bytes := make([]byte, usedBufLen)
		buffer := (*C.UInt8)(unsafe.Pointer(&bytes[0]))
		if C.CFStringGetBytes(ref, cfRange, enc, 0, C.false, buffer, usedBufLen, nil) > 0 {
			header := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
			sh := &reflect.StringHeader{
				Data: header.Data,
				Len:  header.Len,
			}
			return *(*string)(unsafe.Pointer(sh))
		}
	}

	return ""
}

func goValueFromCFPlistRef(ref C.CFPropertyListRef) interface{} {
	if C.CFTypeRef(ref) == 0 {
		return "Unknown"
	}
	switch typeID := C.CFGetTypeID(C.CFTypeRef(ref)); typeID {
	case C.CFBooleanGetTypeID():
		return GoBoolean(C.CFBooleanRef(ref))
	case C.CFNumberGetTypeID():
		return GoInt(C.CFNumberRef(ref))
	case C.CFStringGetTypeID():
		return GoString(C.CFStringRef(ref))
	default:
		panic(fmt.Sprintf("unknown CF type id %v", typeID))
	}
}
