
#include "handler.h"

@implementation CustomProtocolConnector
+ (void)handleGetURLEvent:(NSAppleEventDescriptor *)event
{
    HandleURL((char*)[[[event paramDescriptorForKeyword:keyDirectObject] stringValue] UTF8String]);
}
@end

void StartURLHandler(void) {
	  NSAppleEventManager *appleEventManager = [NSAppleEventManager sharedAppleEventManager];
    [appleEventManager setEventHandler:[CustomProtocolConnector class]
        andSelector:@selector(handleGetURLEvent:)
        forEventClass:kInternetEventClass andEventID:kAEGetURL];
}
