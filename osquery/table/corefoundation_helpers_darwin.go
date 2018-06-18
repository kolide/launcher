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

func copyValue(key, domain, username string) C.CFPropertyListRef {
	return C.CFPreferencesCopyValue(
		CFStringRef(key),
		CFStringRef(domain),
		CFStringRef(username),
		C.kCFPreferencesAnyHost,
	)
}

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

func fromCFPlistRef(ref C.CFPropertyListRef) interface{} {
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
