//go:build darwin
// +build darwin

#import <LocalAuthentication/LocalAuthentication.h>
#include <stdint.h>
#include <stdlib.h>
#include "auth.h"

struct AuthResult Authenticate(char const* reason, int64_t timeout_ns) {
    struct AuthResult authResult;
    LAContext *myContext = [[LAContext alloc] init];
    NSError *authError = nil;
    dispatch_semaphore_t sema = dispatch_semaphore_create(0);
    NSString *nsReason = [NSString stringWithUTF8String:reason];
    __block bool success = false;
    // Use a C-allocated string to hold the error message. This avoids
    // referencing autoreleased/Objective-C managed NSString objects outside
    // the block's lifetime which can lead to use-after-free crashes.
    __block char *errorMessage = NULL;
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
                    const char *cstr = [[error localizedDescription] UTF8String];
                    if (cstr != NULL) {
                        errorMessage = strdup(cstr);
                    }
                }
                dispatch_semaphore_signal(sema);
            }];
    } else {
        success = false; // Cannot evaluate policy
        errorCode = (int)[authError code];
        const char *cstr = [[authError localizedDescription] UTF8String];
        if (cstr != NULL) {
            errorMessage = strdup(cstr);
        }
    }

    // wait for the authentication to complete or timeout
    dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, timeout_ns);
    long result = dispatch_semaphore_wait(sema, timeout);

    if (result != 0) {  // timed out
        [myContext invalidate]; // dismiss presence detection dialog
        success = false;
        errorCode = -1;
        const char *cstr = [@"presence detection timed out" UTF8String];
        if (cstr != NULL) {
            // Only set if we haven't already set an error message
            if (errorMessage == NULL) {
                errorMessage = strdup(cstr);
            }
        }
    }

    dispatch_release(sema);

    authResult.success = success;
    authResult.error_code = errorCode;
    if (errorMessage != NULL) {
        authResult.error_msg = errorMessage; // already strdup'd
    } else {
        authResult.error_msg = NULL;
    }

    return authResult;
}
