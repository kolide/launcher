//go:build darwin
// +build darwin

#import <Foundation/Foundation.h>

BOOL registerAppBundle(char *cAppBundlePath) {
    NSString *appBundlePath = [NSString stringWithUTF8String:cAppBundlePath];
    CFURLRef url = (CFURLRef)[NSURL fileURLWithPath:appBundlePath];

    // Set inUpdate to true to ensure app gets updated
    OSStatus status = LSRegisterURL((CFURLRef)url, YES);

    if (status != noErr) {
        NSLog(@"could not register app bundle: LSRegisterURL returned error: %jd", (intmax_t)status);
        return NO;
    }

    return YES;
}
