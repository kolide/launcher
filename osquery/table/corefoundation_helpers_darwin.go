package table

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#include <CoreFoundation/CoreFoundation.h>
*/
import "C"
import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"reflect"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
)

// Functions with "Create" or "Copy" in the name return references that need to
// be CFReleased. See
// https://developer.apple.com/library/archive/documentation/CoreFoundation/Conceptual/CFMemoryMgmt/Concepts/Ownership.html#//apple_ref/doc/uid/20001148-103029

func execPreferenceAsUser(ctx context.Context, username, key, domain string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, os.Args[0], "cf_preference", key, domain)

	current, err := user.Current()
	if err != nil {
		return nil, errors.Wrap(err, "getting current user for exec")
	}

	if current.Uid == "0" {
		usr, err := user.Lookup(username)
		if err != nil {
			return nil, errors.Wrapf(err, "looking up username %s", username)
		}

		uid, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return nil, errors.Wrapf(err, "converting user uid %s to int", usr.Uid)
		}

		gid, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return nil, errors.Wrapf(err, "converting user gid %s to int", usr.Gid)
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	}

	out, err := cmd.CombinedOutput()
	return bytes.TrimSpace(out), err
}

// PrintPreferenceValue calls the CoreFoundation API to get a preference value and prints to stdout.
// Used by the cf_preference entrypoint when dropping privileges from root to a user context.
func PrintPreferenceValue(key, domain string) {
	keyCFString := cFStringRef(key)
	defer C.CFRelease((C.CFTypeRef)(keyCFString))

	domainCFString := cFStringRef(domain)
	defer C.CFRelease((C.CFTypeRef)(domainCFString))

	val := C.CFPreferencesCopyAppValue(keyCFString, domainCFString)
	if C.CFTypeRef(val) != 0 {
		// will panic if the is NULL
		defer C.CFRelease((C.CFTypeRef)(val))
	}

	fmt.Println(goValueFromCFPlistRef(val))
}

func copyPreferenceValue(key, domain, username string) interface{} {
	keyCFString := cFStringRef(key)
	defer C.CFRelease((C.CFTypeRef)(keyCFString))
	domainCFString := cFStringRef(domain)
	defer C.CFRelease((C.CFTypeRef)(domainCFString))
	usernameCFString := cFStringRef(username)
	defer C.CFRelease((C.CFTypeRef)(usernameCFString))

	val := C.CFPreferencesCopyValue(
		keyCFString, domainCFString, usernameCFString, C.kCFPreferencesAnyHost,
	)
	if C.CFTypeRef(val) != 0 {
		// will panic if the is NULL
		defer C.CFRelease((C.CFTypeRef)(val))
	}
	return goValueFromCFPlistRef(val)
}

// cFStringRef returns a C.CFStringRef which must be released with C.CFRelease
func cFStringRef(s string) C.CFStringRef {
	return C.CFStringCreateWithCString(C.kCFAllocatorDefault, C.CString(s), C.kCFStringEncodingUTF8)
}

func goBoolean(ref C.CFBooleanRef) bool {
	return ref == C.kCFBooleanTrue
}

func goInt(ref C.CFNumberRef) int {
	var n int
	C.CFNumberGetValue(ref, C.CFNumberGetType(ref), unsafe.Pointer(&n))
	return n
}

func goString(ref C.CFStringRef) string {
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
		return goBoolean(C.CFBooleanRef(ref))
	case C.CFNumberGetTypeID():
		return goInt(C.CFNumberRef(ref))
	case C.CFStringGetTypeID():
		return goString(C.CFStringRef(ref))
	default:
		panic(fmt.Sprintf("unknown CF type id %v", typeID))
	}
}
