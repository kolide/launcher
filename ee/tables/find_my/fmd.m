#include "fmd.h"

#import <Cocoa/Cocoa.h>
#import <FMDFMMManager.h>

void getFMDSettings(int* findMyMacEnabled) {
  NSBundle* fmdBundle;
  fmdBundle = [NSBundle bundleWithPath: @"/System/Library/PrivateFrameworks/FindMyDevice.framework"];
  [fmdBundle load];

  Class FMDFMMManager = [fmdBundle classNamed:@"FMDFMMManager"];
  id manager = [FMDFMMManager sharedInstance];

  *findMyMacEnabled = [manager isFMMEnabled] ? 1 : 0;

  return;
}
