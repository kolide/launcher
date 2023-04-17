//go:build darwin
// +build darwin

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#import <Foundation/NSBackgroundActivityScheduler.h>

// Go callbacks
extern void performTask(char*);

void schedule(char* cIdentifier, int repeats) {
  ///*, bool repeats, uint64_t interval*/
  @autoreleasepool {
    [NSApplication sharedApplication];

    NSString* identifier = [NSString stringWithUTF8String:cIdentifier];
    NSBackgroundActivityScheduler* activity =
        [[NSBackgroundActivityScheduler alloc] initWithIdentifier:identifier];

    activity.repeats = repeats ? YES: NO;
    activity.interval = 10;
    activity.qualityOfService = NSQualityOfServiceUserInteractive;
    //   activity.tolerance = 1;

    [activity
        scheduleWithBlock:^(NSBackgroundActivityCompletionHandler completion) {
          performTask(cIdentifier);
          completion(NSBackgroundActivityResultFinished);
        }];

    return; // activity;
  }
}

void stop(void* p) {
  // NSBackgroundActivityScheduler* activity = (NSBackgroundActivityScheduler*)p;
  // [activity invalidate];
}