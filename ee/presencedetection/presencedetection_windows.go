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

// Values for result come from https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.ui.userconsentverificationresult?view=winrt-26100
const resultVerified = uintptr(0)

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

	// Get the window handle from systray
	windowHwnd, err := systray.WindowHandle()
	if err != nil {
		return fmt.Errorf("getting current window handle: %w", err)
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

	if uintptr(resPtr) == resultVerified {
		return nil
	}

	return fmt.Errorf("RequestVerificationForWindowAsync failed with result %+v", resPtr)
}
