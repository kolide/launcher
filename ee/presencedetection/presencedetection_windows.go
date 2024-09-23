//go:build windows
// +build windows

package presencedetection

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/kolide/kit/ulid"
	"github.com/saltosystems/winrt-go"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/storage/streams"
)

// GUIDs retrieved from:
// https://github.com/tpn/winsdk-10/blob/master/Include/10.0.14393.0/winrt/windows.security.credentials.idl
var (
	keyCredentialManagerGuid           = ole.NewGUID("6AAC468B-0EF1-4CE0-8290-4106DA6A63B5")
	keyCredentialRetrievalResultGuid   = ole.NewGUID("58CD7703-8D87-4249-9B58-F6598CC9644E")
	keyCredentialGuid                  = ole.NewGUID("9585EF8D-457B-4847-B11A-FA960BBDB138")
	keyCredentialAttestationResultGuid = ole.NewGUID("78AAB3A1-A3C1-4103-B6CC-472C44171CBB")
)

// Signatures were generated following the guidance in
// https://learn.microsoft.com/en-us/uwp/winrt-cref/winrt-type-system#guid-generation-for-parameterized-types.
// The GUIDs themselves came from the same source as above (windows.security.credentials.idl).
// The GUIDs must be lowercase in the parameterized types.
const (
	keyCredentialRetrievalResultSignature   = "rc(Windows.Security.Credentials.KeyCredentialRetrievalResult;{58cd7703-8d87-4249-9b58-f6598cc9644e})"
	keyCredentialAttestationResultSignature = "rc(Windows.Security.Credentials.KeyCredentialAttestationResult;{78aab3a1-a3c1-4103-b6cc-472c44171cbb})"
	booleanSignature                        = "b1"
)

// KeyCredentialManager is defined here:
// https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.keycredentialmanager?view=winrt-26100
type KeyCredentialManager struct {
	ole.IInspectable
}

func (v *KeyCredentialManager) VTable() *KeyCredentialManagerVTable {
	return (*KeyCredentialManagerVTable)(unsafe.Pointer(v.RawVTable))
}

type KeyCredentialManagerVTable struct {
	ole.IInspectableVtbl
	IsSupportedAsync      uintptr
	RenewAttestationAsync uintptr
	RequestCreateAsync    uintptr
	OpenAsync             uintptr
	DeleteAsync           uintptr
}

// KeyCredentialRetrievalResult is defined here:
// https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.keycredentialretrievalresult?view=winrt-26100
type KeyCredentialRetrievalResult struct {
	ole.IInspectable
}

func (v *KeyCredentialRetrievalResult) VTable() *KeyCredentialRetrievalResultVTable {
	return (*KeyCredentialRetrievalResultVTable)(unsafe.Pointer(v.RawVTable))
}

type KeyCredentialRetrievalResultVTable struct {
	ole.IInspectableVtbl
	GetCredential uintptr
	GetStatus     uintptr
}

// KeyCredential is defined here:
// https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.keycredential?view=winrt-26100
type KeyCredential struct {
	ole.IInspectable
}

func (v *KeyCredential) VTable() *KeyCredentialVTable {
	return (*KeyCredentialVTable)(unsafe.Pointer(v.RawVTable))
}

type KeyCredentialVTable struct {
	ole.IInspectableVtbl
	GetName                              uintptr
	RetrievePublicKeyWithDefaultBlobType uintptr
	RetrievePublicKeyWithBlobType        uintptr
	RequestSignAsync                     uintptr
	GetAttestationAsync                  uintptr
}

// KeyCredentialAttestationResult is defined here:
// https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.keycredentialattestationresult?view=winrt-26100
type KeyCredentialAttestationResult struct {
	ole.IInspectable
}

func (v *KeyCredentialAttestationResult) VTable() *KeyCredentialAttestationResultVTable {
	return (*KeyCredentialAttestationResultVTable)(unsafe.Pointer(v.RawVTable))
}

type KeyCredentialAttestationResultVTable struct {
	ole.IInspectableVtbl
	GetCertificateChainBuffer uintptr
	GetAttestationBuffer      uintptr
	GetStatus                 uintptr
}

// Detect prompts the user via Hello.
// TODO RM:
// * the syscalls panic easily; we will probably need to wrap this in a recovery routine
// * for readability, we should refactor individual calls into functions hanging off the appropriate structs above
func Detect(reason string) (bool, error) {
	if err := ole.RoInitialize(1); err != nil {
		return false, fmt.Errorf("initializing: %w", err)
	}

	// Get access to the KeyCredentialManager
	factory, err := ole.RoGetActivationFactory("Windows.Security.Credentials.KeyCredentialManager", ole.IID_IInspectable)
	if err != nil {
		return false, fmt.Errorf("getting activation factory for KeyCredentialManager: %w", err)
	}
	defer factory.Release()
	managerObj, err := factory.QueryInterface(keyCredentialManagerGuid)
	if err != nil {
		return false, fmt.Errorf("getting KeyCredentialManager from factory: %w", err)
	}
	defer managerObj.Release()
	keyCredentialManager := (*KeyCredentialManager)(unsafe.Pointer(managerObj))

	// Check to see if Hello is an option
	isHelloSupported, err := isSupported(keyCredentialManager)
	if err != nil {
		return false, fmt.Errorf("determining whether Hello is supported: %w", err)
	}
	if !isHelloSupported {
		return false, errors.New("Hello is not supported")
	}

	// Create a credential that will be tied to the current user and this application
	credentialName := ulid.New()
	_, keyCredentialObj, err := requestCreate(keyCredentialManager, credentialName)
	defer func() {
		if keyCredentialObj != nil {
			keyCredentialObj.Release()
		}
	}()
	if err != nil {
		return false, fmt.Errorf("creating credential, %w", err)
	}

	credential := (*KeyCredential)(unsafe.Pointer(keyCredentialObj))
	if _, err := getAttestationAsync(credential); err != nil {
		return false, fmt.Errorf("getting attestation from credential: %w", err)
	}

	return true, nil
}

// isSupported calls Windows.Security.Credentials.KeyCredentialManager.IsSupportedAsync.
// It determines whether the current device and user is capable of provisioning a key credential.
// See: https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.keycredentialmanager.issupportedasync?view=winrt-26100
func isSupported(keyCredentialManager *KeyCredentialManager) (bool, error) {
	var isSupportedAsyncOperation *foundation.IAsyncOperation
	ret, _, _ := syscall.SyscallN(
		keyCredentialManager.VTable().IsSupportedAsync,
		0, // Because this is a static function, we don't pass in a reference to `this`
		uintptr(unsafe.Pointer(&isSupportedAsyncOperation)), // Windows.Foundation.IAsyncOperation<boolean>
	)
	if ret != 0 {
		return false, fmt.Errorf("calling IsSupportedAsync: %w", ole.NewError(ret))
	}

	// IsSupportedAsync returns Windows.Foundation.IAsyncOperation<boolean>
	iid := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, booleanSignature)
	statusChan := make(chan foundation.AsyncStatus)
	handler := foundation.NewAsyncOperationCompletedHandler(ole.NewGUID(iid), func(instance *foundation.AsyncOperationCompletedHandler, asyncInfo *foundation.IAsyncOperation, asyncStatus foundation.AsyncStatus) {
		statusChan <- asyncStatus
	})
	defer handler.Release()
	isSupportedAsyncOperation.SetCompleted(handler)

	select {
	case operationStatus := <-statusChan:
		if operationStatus != foundation.AsyncStatusCompleted {
			return false, fmt.Errorf("IsSupportedAsync operation did not complete: status %d", operationStatus)
		}
	case <-time.After(5 * time.Second):
		return false, errors.New("timed out waiting for IsSupportedAsync operation to complete")
	}

	res, err := isSupportedAsyncOperation.GetResults()
	if err != nil {
		return false, fmt.Errorf("getting results of IsSupportedAsync: %w", err)
	}

	return uintptr(res) > 0, nil
}

// requestCreate calls Windows.Security.Credentials.KeyCredentialManager.RequestCreateAsync.
// It creates a new key credential for the current user and application.
// See: https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.keycredentialmanager.requestcreateasync?view=winrt-26100
func requestCreate(keyCredentialManager *KeyCredentialManager, credentialName string) ([]byte, *ole.IDispatch, error) {
	credentialNameHString, err := ole.NewHString(credentialName)
	if err != nil {
		return nil, nil, fmt.Errorf("creating credential name hstring: %w", err)
	}
	defer ole.DeleteHString(credentialNameHString)

	var requestCreateAsyncOperation *foundation.IAsyncOperation
	requestCreateReturn, _, _ := syscall.SyscallN(
		keyCredentialManager.VTable().RequestCreateAsync,
		0, // Because this is a static function, we don't pass in a reference to `this`
		uintptr(unsafe.Pointer(&credentialNameHString)), // The name of the key credential to create
		0, // KeyCredentialCreationOption -- 0 indicates to replace any existing key credentials, 1 indicates to fail if a key credential already exists
		uintptr(unsafe.Pointer(&requestCreateAsyncOperation)), // Windows.Foundation.IAsyncOperation<KeyCredentialRetrievalResult>
	)
	if requestCreateReturn != 0 {
		return nil, nil, fmt.Errorf("calling RequestCreateAsync: %w", ole.NewError(requestCreateReturn))
	}

	// RequestCreateAsync returns Windows.Foundation.IAsyncOperation<KeyCredentialRetrievalResult>
	iid := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, keyCredentialRetrievalResultSignature)
	statusChan := make(chan foundation.AsyncStatus)
	handler := foundation.NewAsyncOperationCompletedHandler(ole.NewGUID(iid), func(instance *foundation.AsyncOperationCompletedHandler, asyncInfo *foundation.IAsyncOperation, asyncStatus foundation.AsyncStatus) {
		statusChan <- asyncStatus
	})
	defer handler.Release()
	requestCreateAsyncOperation.SetCompleted(handler)

	select {
	case operationStatus := <-statusChan:
		if operationStatus != foundation.AsyncStatusCompleted {
			return nil, nil, fmt.Errorf("RequestCreateAsync operation did not complete: status %d", operationStatus)
		}
	case <-time.After(1 * time.Minute):
		return nil, nil, errors.New("timed out waiting for RequestCreateAsync operation to complete")
	}

	// Retrieve the results from the async operation
	resPtr, err := requestCreateAsyncOperation.GetResults()
	if err != nil {
		return nil, nil, fmt.Errorf("getting results of RequestCreateAsync: %w", err)
	}

	if uintptr(resPtr) == 0x0 {
		return nil, nil, errors.New("no response to RequestCreateAsync")
	}

	resultObj, err := (*ole.IUnknown)(resPtr).QueryInterface(keyCredentialRetrievalResultGuid)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get KeyCredentialRetrievalResult from result of RequestCreateAsync: %w", err)
	}
	defer resultObj.Release()
	result := (*KeyCredentialRetrievalResult)(unsafe.Pointer(resultObj))

	// Now, retrieve the KeyCredential from the KeyCredentialRetrievalResult
	var credentialPointer unsafe.Pointer
	getCredentialReturn, _, _ := syscall.SyscallN(
		result.VTable().GetCredential,
		uintptr(unsafe.Pointer(result)), // Since we're retrieving an object property, we need a reference to `this`
		uintptr(unsafe.Pointer(&credentialPointer)),
	)
	if getCredentialReturn != 0 {
		return nil, nil, fmt.Errorf("calling GetCredential on KeyCredentialRetrievalResult: %w", ole.NewError(getCredentialReturn))
	}

	keyCredentialObj, err := (*ole.IUnknown)(credentialPointer).QueryInterface(keyCredentialGuid)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get KeyCredential from KeyCredentialRetrievalResult: %w", err)
	}
	defer keyCredentialObj.Release()
	credential := (*KeyCredential)(unsafe.Pointer(keyCredentialObj))

	// All right, things are going swimmingly. Let's retrieve the public key.
	var pubkeyBufferPointer unsafe.Pointer
	retrievePubKeyReturn, _, _ := syscall.SyscallN(
		credential.VTable().RetrievePublicKeyWithDefaultBlobType,
		uintptr(unsafe.Pointer(credential)), // Not a static method, so we need a reference to `this`
		uintptr(unsafe.Pointer(&pubkeyBufferPointer)),
	)
	if retrievePubKeyReturn != 0 {
		return nil, nil, fmt.Errorf("calling RetrievePublicKey on KeyCredential: %w", ole.NewError(retrievePubKeyReturn))
	}

	pubkeyBufferObj, err := (*ole.IUnknown)(pubkeyBufferPointer).QueryInterface(ole.NewGUID(streams.GUIDIBuffer))
	if err != nil {
		return nil, nil, fmt.Errorf("could not get buffer from result of RetrievePublicKey: %w", err)
	}
	defer pubkeyBufferObj.Release()
	pubkeyBuffer := (*streams.IBuffer)(unsafe.Pointer(pubkeyBufferObj))

	pubkeyBufferLen, err := pubkeyBuffer.GetLength()
	if err != nil {
		return nil, nil, fmt.Errorf("could not get length of pubkey buffer: %w", err)
	}
	pubkeyReader, err := streams.DataReaderFromBuffer(pubkeyBuffer)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create data reader for pubkey buffer: %w", err)
	}
	pubkeyBytes, err := pubkeyReader.ReadBytes(pubkeyBufferLen)
	if err != nil {
		return nil, nil, fmt.Errorf("reading from pubkey buffer: %w", err)
	}

	return pubkeyBytes, keyCredentialObj, nil
}

// getAttestationAsync calls Windows.Security.Credentials.KeyCredential.GetAttestationAsync.
// It gets an attestation for a key credential.
// See: https://learn.microsoft.com/en-us/uwp/api/windows.security.credentials.keycredential.getattestationasync?view=winrt-26100
func getAttestationAsync(credential *KeyCredential) ([]byte, error) {
	// Now it's time to get the attestation. This is another async operation.
	var getAttestationAsyncOperation *foundation.IAsyncOperation
	getAttestationReturn, _, _ := syscall.SyscallN(
		credential.VTable().GetAttestationAsync,
		uintptr(unsafe.Pointer(credential)),                    // Not a static method, so we need a reference to `this`
		uintptr(unsafe.Pointer(&getAttestationAsyncOperation)), // Windows.Foundation.IAsyncOperation<KeyCredentialAttestationResult>
	)
	if getAttestationReturn != 0 {
		return nil, fmt.Errorf("calling GetAttestationAsync: %w", ole.NewError(getAttestationReturn))
	}

	// GetAttestationAsync returns Windows.Foundation.IAsyncOperation<KeyCredentialAttestationResult>
	attestionResultIid := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, keyCredentialAttestationResultSignature)
	attestationStatusChan := make(chan foundation.AsyncStatus)
	attestationHandler := foundation.NewAsyncOperationCompletedHandler(ole.NewGUID(attestionResultIid), func(instance *foundation.AsyncOperationCompletedHandler, asyncInfo *foundation.IAsyncOperation, asyncStatus foundation.AsyncStatus) {
		attestationStatusChan <- asyncStatus
	})
	defer attestationHandler.Release()
	getAttestationAsyncOperation.SetCompleted(attestationHandler)

	select {
	case operationStatus := <-attestationStatusChan:
		if operationStatus != foundation.AsyncStatusCompleted {
			return nil, fmt.Errorf("GetAttestationAsync operation did not complete: status %d", operationStatus)
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("timed out waiting for GetAttestationAsync operation to complete")
	}

	// Retrieve the results from the async attestation operation
	attestationResPtr, err := getAttestationAsyncOperation.GetResults()
	if err != nil {
		return nil, fmt.Errorf("getting results of GetAttestationAsync: %w", err)
	}

	if uintptr(attestationResPtr) == 0x0 {
		return nil, errors.New("no response to GetAttestationAsync")
	}

	attestationResultObj, err := (*ole.IUnknown)(attestationResPtr).QueryInterface(keyCredentialAttestationResultGuid)
	if err != nil {
		return nil, fmt.Errorf("could not get KeyCredentialAttestationResult from result of GetAttestationAsync: %w", err)
	}
	defer attestationResultObj.Release()
	attestationResult := (*KeyCredentialAttestationResult)(unsafe.Pointer(attestationResultObj))

	// From here, we can retrieve both the attestation (via GetAttestationBuffer) and the certificate chain (via GetCertificateChainBuffer).
	// Both of these operations should look identical to our IBuffer usage above, so I'm just going to grab the attestation here
	// for now and fill in the certificate chain if we happen to need it later.
	var attestationBufferPointer unsafe.Pointer
	getAttestationBufferReturn, _, _ := syscall.SyscallN(
		attestationResult.VTable().GetAttestationBuffer,
		uintptr(unsafe.Pointer(attestationResult)), // Not a static method, so we need a reference to `this`
		uintptr(unsafe.Pointer(&attestationBufferPointer)),
	)
	if getAttestationBufferReturn != 0 {
		return nil, fmt.Errorf("calling GetAttestationBuffer on KeyCredentialAttestationResult: %w", ole.NewError(getAttestationBufferReturn))
	}

	attestationBufferObj, err := (*ole.IUnknown)(attestationBufferPointer).QueryInterface(ole.NewGUID(streams.GUIDIBuffer))
	if err != nil {
		return nil, fmt.Errorf("could not get buffer from result of GetAttestationBuffer: %w", err)
	}
	defer attestationBufferObj.Release()
	attestationBuffer := (*streams.IBuffer)(unsafe.Pointer(attestationBufferObj))

	attestationBufferLen, err := attestationBuffer.GetLength()
	if err != nil {
		return nil, fmt.Errorf("could not get length of attestation buffer: %w", err)
	}
	attestationReader, err := streams.DataReaderFromBuffer(attestationBuffer)
	if err != nil {
		return nil, fmt.Errorf("could not create data reader for attestation buffer: %w", err)
	}
	attestationBytes, err := attestationReader.ReadBytes(attestationBufferLen)
	if err != nil {
		return nil, fmt.Errorf("reading from attestation buffer: %w", err)
	}

	return attestationBytes, nil
}

/*
// waitForAsyncOperation should allow us to abstract away the details of waiting for an async operation,
// but right now it only works for IsSupportedAsync; it results in an error 3 being returned from RequestCreateAsync.
// TODO RM -- fix.
func waitForAsyncOperation(signature string, timeout time.Duration, asyncOperation *foundation.IAsyncOperation) (unsafe.Pointer, error) {
	statusChan := make(chan foundation.AsyncStatus)

	iid := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, signature)
	handler := foundation.NewAsyncOperationCompletedHandler(ole.NewGUID(iid), func(instance *foundation.AsyncOperationCompletedHandler, asyncInfo *foundation.IAsyncOperation, asyncStatus foundation.AsyncStatus) {
		statusChan <- asyncStatus
	})
	defer handler.Release()

	asyncOperation.SetCompleted(handler)

	select {
	case operationStatus := <-statusChan:
		if operationStatus != foundation.AsyncStatusCompleted {
			return nil, fmt.Errorf("async operation did not complete: status %d", operationStatus)
		}
	case <-time.After(timeout):
		return nil, errors.New("timed out waiting for operation to complete")
	}

	return asyncOperation.GetResults()
}
*/
