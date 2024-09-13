package presence

/*
#cgo CFLAGS: -x objective-c -fmodules -fblocks
#cgo LDFLAGS: -framework CoreFoundation -framework LocalAuthentication -framework Foundation
#include <stdlib.h>
#include <stdio.h>
#import <LocalAuthentication/LocalAuthentication.h>

struct AuthResult {
    bool success;       // true for success, false for failure
    char* error_msg;    // Error message if any
    int error_code;     // Error code if any
};

struct AuthResult Authenticate(char const* reason) {
    struct AuthResult authResult;
    LAContext *myContext = [[LAContext alloc] init];
    NSError *authError = nil;
    dispatch_semaphore_t sema = dispatch_semaphore_create(0);
    NSString *nsReason = [NSString stringWithUTF8String:reason];
    __block bool success = false;
    __block NSString *errorMessage = nil;
    __block int errorCode = 0;

    // Use LAPolicyDeviceOwnerAuthentication to allow biometrics and password fallback
    if ([myContext canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:&authError]) {
        [myContext evaluatePolicy:LAPolicyDeviceOwnerAuthentication
            localizedReason:nsReason
            reply:^(BOOL policySuccess, NSError *error) {
                if (policySuccess) {
                    success = true; // Authentication successful
                } else {
                    success = false;
                    errorCode = (int)[error code];
                    errorMessage = [error localizedDescription];
                    if (error.code == LAErrorUserFallback || error.code == LAErrorAuthenticationFailed) {
                        // Prompting for password
                        [myContext evaluatePolicy:LAPolicyDeviceOwnerAuthentication
                            localizedReason:@"Please enter your password"
                            reply:^(BOOL pwdSuccess, NSError *error) {
                                if (pwdSuccess) {
                                    success = true;
                                } else {
                                    success = false;
                                    errorCode = (int)[error code];
                                    errorMessage = [error localizedDescription];
                                }
                                dispatch_semaphore_signal(sema);
                            }];
                    } else {
                        errorCode = (int)[error code];
                        errorMessage = [error localizedDescription];
                    }
                }
                dispatch_semaphore_signal(sema);
            }];
    } else {
        success = false; // Cannot evaluate policy
        errorCode = (int)[authError code];
        errorMessage = [authError localizedDescription];
    }

    dispatch_semaphore_wait(sema, DISPATCH_TIME_FOREVER);
    dispatch_release(sema);

    authResult.success = success;
    authResult.error_code = errorCode;
    if (errorMessage != nil) {
        authResult.error_msg = strdup([errorMessage UTF8String]); // Copy error message to C string
    } else {
        authResult.error_msg = NULL;
    }

    return authResult;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func detect(reason string) (bool, error) {
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
