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
	"github.com/kolide/systray"
	"github.com/saltosystems/winrt-go"
	"github.com/saltosystems/winrt-go/windows/foundation"
)

// GUIDs retrieved from:
// https://github.com/tpn/winsdk-10/blob/master/Include/10.0.16299.0/um/UserConsentVerifierInterop.idl
var (
	iUserConsentVerifierStaticsGuid = ole.NewGUID("AF4F3F91-564C-4DDC-B8B5-973447627C65") // Windows.Security.Credentials.UI.UserConsentVerifier
	iUserConsentVerifierInteropGuid = ole.NewGUID("39E050C3-4E74-441A-8DC0-B81104DF949C") // UserConsentVerifierInterop
)

// Signatures were generated following the guidance in
// https://learn.microsoft.com/en-us/uwp/winrt-cref/winrt-type-system#guid-generation-for-parameterized-types.
const (
	userConsentVerificationResultSignature = "enum(Windows.Security.Credentials.UI.UserConsentVerificationResult;i4)" // i4 is underlying type of int32
)

// Values for result come from https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.ui.userconsentverificationresult?view=winrt-26100
const (
	resultVerified             uintptr = 0x0
	resultDeviceNotPresent     uintptr = 0x1
	resultNotConfiguredForUser uintptr = 0x2
	resultDisabledByPolicy     uintptr = 0x3
	resultDeviceBusy           uintptr = 0x4
	resultRetriesExhausted     uintptr = 0x5
	resultCanceled             uintptr = 0x6
)

var resultErrorMessageMap = map[uintptr]string{
	resultDeviceNotPresent:     "There is no authentication device available.",
	resultNotConfiguredForUser: "An authentication verifier device is not configured for this user.",
	resultDisabledByPolicy:     "Group policy has disabled authentication device verification.",
	resultDeviceBusy:           "The authentication device is performing an operation and is unavailable.",
	resultRetriesExhausted:     "After 10 attempts, the original verification request and all subsequent attempts at the same verification were not verified.",
	resultCanceled:             "The verification operation was canceled.",
}

// IUserConsentVerifierInterop is the interop interface for UserConsentVerifier. Both are documented here:
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

var roInitialize = sync.OnceFunc(func() {
	// Call ole.RoInitialize(1) only once
	ole.RoInitialize(1)
})

// Detect prompts the user via Hello.
func Detect(reason string, timeout time.Duration) (bool, error) {
	roInitialize()

	if err := requestVerification(reason, timeout); err != nil {
		return false, fmt.Errorf("requesting verification: %w", err)
	}

	return true, nil
}

// requestVerification calls Windows.Security.Credentials.UI.UserConsentVerifier.RequestVerificationAsync via the interop interface.
// See: https://learn.microsoft.com/en-us/windows/win32/api/userconsentverifierinterop/nf-userconsentverifierinterop-iuserconsentverifierinterop-requestverificationforwindowasync
func requestVerification(reason string, timeout time.Duration) error {
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

	// Get the current window handle (a HWND) from systray
	windowHandle, err := systray.WindowHandle()
	if err != nil {
		return fmt.Errorf("getting current window handle: %w", err)
	}

	// Create hstring for "reason" message
	reasonHString, err := ole.NewHString(reason)
	if err != nil {
		return fmt.Errorf("creating reason hstring: %w", err)
	}
	defer ole.DeleteHString(reasonHString)

	// RequestVerificationForWindowAsync returns Windows.Foundation.IAsyncOperation<UserConsentVerificationResult>
	// -- prepare the return values.
	refiid := winrt.ParameterizedInstanceGUID(foundation.GUIDIAsyncOperation, userConsentVerificationResultSignature)
	var requestVerificationAsyncOperation *foundation.IAsyncOperation

	// In a lot of places, when passing HSTRINGs into `syscall.SyscallN`, we see it passed in
	// as `uintptr(unsafe.Pointer(&reasonHString))`. However, this does not work for
	// RequestVerificationForWindowAsync -- the window displays an incorrectly-encoded message.
	//
	// We CAN pass the message in as `uintptr(unsafe.Pointer(reasonHString))` -- this works, and
	// is a choice that the ole library makes in several places (see RoActivateInstance,
	// RoGetActiviationFactory). However, Golang's unsafeptr analysis flags this as potentially
	// unsafe use of a pointer.
	//
	// To avoid the unsafeptr warning, we therefore pass in the message as simply `uintptr(reasonHString)`.
	// This is safe and effective.

	requestVerificationReturn, _, _ := syscall.SyscallN(
		verifier.VTable().RequestVerificationForWindowAsync,
		uintptr(unsafe.Pointer(verifier)),                           // Reference to our interop
		uintptr(windowHandle),                                       // HWND to our application's window
		uintptr(reasonHString),                                      // The message to include in the verification request
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
	case <-time.After(timeout):
		return errors.New("timed out waiting for RequestVerificationForWindowAsync operation to complete")
	}

	// Retrieve the results from the async operation
	resPtr, err := requestVerificationAsyncOperation.GetResults()
	if err != nil {
		return fmt.Errorf("getting results of RequestVerificationForWindowAsync: %w", err)
	}

	verificationResult := uintptr(resPtr)
	if verificationResult == resultVerified {
		return nil
	}

	if errMsg, ok := resultErrorMessageMap[verificationResult]; ok {
		return fmt.Errorf("requesting verification failed: %s", errMsg)
	}

	return fmt.Errorf("RequestVerificationForWindowAsync failed with unknown result %+v", verificationResult)
}
