//go:build darwin
// +build darwin

#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>

// Go callback
extern void handleUniversalLink(char*);

@interface UniversalLinkConnector : NSObject <NSApplicationDelegate>
@end
@implementation UniversalLinkConnector
- (BOOL)application:(NSApplication *)application continueUserActivity:(NSUserActivity *)userActivity restorationHandler:(void (^)(NSArray<id<NSUserActivityRestoring>> *restorableObjects))restorationHandler {
    NSLog(@"TODO RM universal_link_handler: Userinfo Data==>%@  useractivityTitle ==>%@  activityType-->%@ webpage-->%@",userActivity.userInfo , userActivity.title,userActivity.activityType,userActivity.webpageURL );
    handleUniversalLink((char*)userActivity.webpageURL);

    return YES;
}
@end

UniversalLinkConnector *universalLinkConnector;
NSApplication *nsApplication;

void StartUniversalLinkHandler(void) {
    @autoreleasepool {
        nsApplication = [NSApplication sharedApplication];

        universalLinkConnector = [[UniversalLinkConnector alloc] init];
        [nsApplication setDelegate:universalLinkConnector];
    }
}
