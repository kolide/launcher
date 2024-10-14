//go:build windows
// +build windows

package presencedetection

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go"
	"github.com/saltosystems/winrt-go/windows/foundation"
)

// GUIDs retrieved from:
// https://github.com/tpn/winsdk-10/blob/master/Include/10.0.16299.0/um/UserConsentVerifierInterop.idl
var (
	iUserConsentVerifierStaticsGuid = ole.NewGUID("AF4F3F91-564C-4DDC-B8B5-973447627C65")
	iUserConsentVerifierInteropGuid = ole.NewGUID("39E050C3-4E74-441A-8DC0-B81104DF949C")
)

// Signatures were generated following the guidance in
// https://learn.microsoft.com/en-us/uwp/winrt-cref/winrt-type-system#guid-generation-for-parameterized-types.
// The GUIDs themselves came from the same source as above (windows.security.credentials.idl).
// The GUIDs must be lowercase in the parameterized types.
const (
	userConsentVerificationResultSignature = "enum(Windows.Security.Credentials.UI.UserConsentVerificationResult;i4)" // i4 is underlying type of int32
)

// UserConsentVerifier is defined here, with references to IUserConsentVerifierInterop below:
// https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.ui.userconsentverifier?view=winrt-26100#desktop-apps-using-cwinrt
type IUserConsentVerifierInterop struct {
	ole.IInspectable
}

func (v *IUserConsentVerifierInterop) VTable() *IUserConsentVerifierInteropVTable {
	return (*IUserConsentVerifierInteropVTable)(unsafe.Pointer(v.RawVTable))
}

type IUserConsentVerifierInteropVTable struct {
	ole.IInspectableVtbl
	RequestVerificationForWindowAsync uintptr
}

// See: https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-wndclassexa
type WNDCLASSEX struct {
	cbSize        uint32 // Size of struct
	style         uint32
	lpfnWndProc   uintptr        // Pointer to the Windows procedure
	cbClsExtra    int32          // The number of extra bytes to allocate following the window-class structure
	cbWndExtra    int32          // The number of extra bytes to allocate following the window instance
	hInstance     syscall.Handle // Handle for the process that will create the window (i.e. launcher.exe)
	hIcon         syscall.Handle // Null for default icon
	hCursor       syscall.Handle // Handle for cursor resource
	hbrBackground syscall.Handle // Color value must be one of the standard system colors with 1 added
	lpszMenuName  *uint16        // Null if no menu
	lpszClassName *uint16        // Identifies this class
	hIconSm       syscall.Handle // Handle to small icon, can also be null
}

const (
	COLOR_WINDOW = 5

	// https://learn.microsoft.com/en-us/windows/win32/winmsg/window-class-styles
	CS_SAVEBITS   = 0x0800
	CS_DROPSHADOW = 0x00020000

	// https://learn.microsoft.com/en-us/windows/win32/winmsg/extended-window-styles
	WS_EX_WINDOWEDGE       = 0x00000100
	WS_EX_CLIENTEDGE       = 0x00000200
	WS_EX_OVERLAPPEDWINDOW = WS_EX_WINDOWEDGE | WS_EX_CLIENTEDGE

	// 	overlappedWindow := 0 | 0x800000 | 0x400000 | 0x80000 | 0x40000 | 0x20000 | 0x10000
	CW_USEDEFAULT       = 0x80000000
	WS_OVERLAPPED       = 0x00000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_THICKFRAME       = 0x00040000
	WS_MINIMIZEBOX      = 0x20000000
	WS_MAXIMIZEBOX      = 0x01000000
	WS_OVERLAPPEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX

	cWM_DESTROY = 0x0002
	cWM_CLOSE   = 0x0010
)

var roInitialize = sync.OnceFunc(func() {
	ole.RoInitialize(1)
})

// Detect prompts the user via Hello.
func Detect(reason string) (bool, error) {
	roInitialize()

	if err := requestVerification(reason); err != nil {
		return false, fmt.Errorf("requesting verification: %w", err)
	}

	return true, nil
}

// requestVerification calls Windows.Security.Credentials.UI.UserConsentVerifier.RequestVerificationAsync.
// See: https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.ui.userconsentverifier.requestverificationasync?view=winrt-26100
func requestVerification(reason string) error {
	// Get access to UserConsentVerifier via factory
	factory, err := ole.RoGetActivationFactory("Windows.Security.Credentials.UI.UserConsentVerifier", iUserConsentVerifierStaticsGuid)
	if err != nil {
		return fmt.Errorf("getting activation factory for UserConsentVerifier: %w", err)
	}
	defer factory.Release()

	// Query for the interop interface, which we need to actually interact with this method
	verifierObj, err := factory.QueryInterface(iUserConsentVerifierInteropGuid)
	if err != nil {
		return fmt.Errorf("getting UserConsentVerifier from factory: %w", err)
	}
	defer verifierObj.Release()
	verifier := (*IUserConsentVerifierInterop)(unsafe.Pointer(verifierObj))

	// Create a window
	windowHwnd, err := createWindow()
	if err != nil {
		return fmt.Errorf("creating window: %w", err)
	}

	// Create hstring for "reason" message
	reasonHString, err := ole.NewHString(reason)
	if err != nil {
		return fmt.Errorf("creating reason hstring: %w", err)
	}
	defer ole.DeleteHString(reasonHString)

	// https://learn.microsoft.com/en-us/windows/win32/api/userconsentverifierinterop/nf-userconsentverifierinterop-iuserconsentverifierinterop-requestverificationforwindowasync
	// RequestVerificationForWindowAsync returns Windows.Foundation.IAsyncOperation<UserConsentVerificationResult>
	refiid := winrt.ParameterizedInstanceGUID(foundation.GUIDIAsyncOperation, userConsentVerificationResultSignature)
	var requestVerificationAsyncOperation *foundation.IAsyncOperation
	requestVerificationReturn, _, _ := syscall.SyscallN(
		verifier.VTable().RequestVerificationForWindowAsync,
		uintptr(unsafe.Pointer(verifier)),                           // Reference to our interop
		uintptr(windowHwnd),                                         // HWND to our window
		uintptr(unsafe.Pointer(&reasonHString)),                     // The message to include in the verification request
		uintptr(unsafe.Pointer(ole.NewGUID(refiid))),                // REFIID -- reference to the interface identifier for the return value (below)
		uintptr(unsafe.Pointer(&requestVerificationAsyncOperation)), // Return value -- Windows.Foundation.IAsyncOperation<UserConsentVerificationResult>
	)
	if requestVerificationReturn != 0 {
		return fmt.Errorf("calling RequestVerificationForWindowAsync: %w", ole.NewError(requestVerificationReturn))
	}

	// Wait for async operation to complete
	iid := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, userConsentVerificationResultSignature)
	statusChan := make(chan foundation.AsyncStatus)
	handler := foundation.NewAsyncOperationCompletedHandler(ole.NewGUID(iid), func(instance *foundation.AsyncOperationCompletedHandler, asyncInfo *foundation.IAsyncOperation, asyncStatus foundation.AsyncStatus) {
		statusChan <- asyncStatus
	})
	defer handler.Release()
	requestVerificationAsyncOperation.SetCompleted(handler)

	select {
	case operationStatus := <-statusChan:
		if operationStatus != foundation.AsyncStatusCompleted {
			return fmt.Errorf("RequestVerificationForWindowAsync operation did not complete: status %d", operationStatus)
		}
	case <-time.After(1 * time.Minute):
		return errors.New("timed out waiting for RequestVerificationForWindowAsync operation to complete")
	}

	// Retrieve the results from the async operation
	resPtr, err := requestVerificationAsyncOperation.GetResults()
	if err != nil {
		return fmt.Errorf("getting results of RequestVerificationForWindowAsync: %w", err)
	}
	if uintptr(resPtr) == 0x0 {
		return errors.New("no response to RequestVerificationForWindowAsync")
	}

	// TODO RM -- switch for enum response
	return fmt.Errorf("response to RequestVerificationForWindowAsync: %+v", resPtr)
}

// createWindow calls CreateWindowExW to create a basic window; it returns the HWND to the window.
// See: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-createwindowexw
func createWindow() (syscall.Handle, error) {
	instance, err := getInstance()
	if err != nil {
		return syscall.InvalidHandle, fmt.Errorf("getting instance: %w", err)
	}

	// TODO RM: need to close handles?

	className := "launcher"
	classHandle, err := registerClass(className, instance)
	if err != nil {
		return syscall.InvalidHandle, fmt.Errorf("registering class: %w", err)
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	procCreateWindowExW := user32.NewProc("CreateWindowExW")

	title, err := syscall.UTF16PtrFromString("Kolide")
	if err != nil {
		return syscall.InvalidHandle, fmt.Errorf("creating title string: %w", err)
	}

	r0, _, e0 := procCreateWindowExW.Call(
		uintptr(0),                     // DWORD dwExStyle
		uintptr(classHandle),           // LPCWSTR lpClassName (allegedly optional)
		uintptr(unsafe.Pointer(title)), // LPCWSTR lpWindowName (optional)
		uintptr(WS_OVERLAPPEDWINDOW),   // DWORD dwStyle
		uintptr(CW_USEDEFAULT),         // int x
		uintptr(CW_USEDEFAULT),         // int y
		uintptr(CW_USEDEFAULT),         // int nWidth
		uintptr(CW_USEDEFAULT),         // int nHeight
		uintptr(0),                     // HWND hWndParent (optional)
		uintptr(0),                     // HMENU hMenu (optional)
		uintptr(instance),              // HINSTANCE hInstance
		uintptr(0))                     // LPVOID lpParam (optional)
	handle := syscall.Handle(r0)
	if handle == 0 {
		return syscall.InvalidHandle, fmt.Errorf("calling CreateWindowExW: %v", e0)
	}

	return syscall.Handle(r0), nil
}

// getInstance calls GetModuleHandleW to get a HINSTANCE reference to the current launcher.exe process.
// See: https://learn.microsoft.com/en-us/windows/win32/api/libloaderapi/nf-libloaderapi-getmodulehandlew
func getInstance() (syscall.Handle, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandleW := kernel32.NewProc("GetModuleHandleW")

	r0, _, e0 := syscall.SyscallN(
		procGetModuleHandleW.Addr(),
		0,
	)
	instanceHandle := syscall.Handle(r0)
	if instanceHandle == 0 {
		return syscall.InvalidHandle, fmt.Errorf("could not get module handle: %v", e0)
	}

	return instanceHandle, nil
}

// registerClass calls RegisterClassExW to register a class with name `className` that can be used
// to create windows.
// See: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-registerclassexw
// Also see:
// - https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-destroywindow
// - https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-postquitmessage
// - https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-defwindowprocw
func registerClass(className string, instance syscall.Handle) (syscall.Handle, error) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procRegisterClassExW := user32.NewProc("RegisterClassExW")
	procDestroyWindow := user32.NewProc("DestroyWindow")
	procPostQuitMessage := user32.NewProc("PostQuitMessage")
	procDefWindowProcW := user32.NewProc("DefWindowProcW")

	classNamePtr, err := syscall.UTF16PtrFromString(className)
	if err != nil {
		return syscall.InvalidHandle, fmt.Errorf("creating pointer to class name: %w", err)
	}

	fn := func(hWnd syscall.Handle, uMsg uint32, wParam, lParam uintptr) uintptr {
		switch uMsg {
		case cWM_CLOSE:
			procDestroyWindow.Call(uintptr(hWnd))
			return 0
		case cWM_DESTROY:
			procPostQuitMessage.Call(uintptr(0))
			return 0
		default:
			r0, _, _ := procDefWindowProcW.Call(
				uintptr(hWnd),
				uintptr(uMsg),
				uintptr(wParam),
				uintptr(lParam),
			)
			return uintptr(r0)
		}
	}

	class := WNDCLASSEX{
		// style:       CS_SAVEBITS | CS_DROPSHADOW,
		lpfnWndProc: syscall.NewCallback(fn),
		hInstance:   instance,
		// hbrBackground: COLOR_WINDOW + 1,
		lpszClassName: classNamePtr,
	}
	class.cbSize = uint32(unsafe.Sizeof(class))

	r0, _, e0 := syscall.SyscallN(
		procRegisterClassExW.Addr(),
		uintptr(unsafe.Pointer(&class)),
	)

	classHandle := syscall.Handle(r0)
	if classHandle == 0 {
		return syscall.InvalidHandle, fmt.Errorf("could not get module handle: %v", e0)
	}

	return classHandle, nil
}
