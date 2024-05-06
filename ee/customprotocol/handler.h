#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>

// Go callback
extern void handleURL(char*);

@interface CustomProtocolConnector : NSObject
+ (void)handleGetURLEvent:(NSAppleEventDescriptor *)event;
@end

void StartURLHandler(void);
