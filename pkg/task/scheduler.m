#include "scheduler.h"

#import <Foundation/Foundation.h>
#import <Foundation/NSBackgroundActivityScheduler.h>

void schedule() {
  NSBackgroundActivityScheduler *activity = [[NSBackgroundActivityScheduler alloc] initWithIdentifier:@"com.example.MyApp.updatecheck"];

  activity.interval = 30 * 60;
  activity.tolerance = 15 * 60;

  [activity
    scheduleWithBlock:^(NSBackgroundActivityCompletionHandler completion) {
      perform();
      // Perform the activity
      completion(NSBackgroundActivityResultFinished);
  }];
}