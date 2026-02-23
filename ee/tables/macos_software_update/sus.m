#include "sus.h"

#import <Cocoa/Cocoa.h>
#import <SUOSUShimController.h>
#import <SUScanController.h>
#import <SUSharedPrefs.h>
#import <SUUpdateProduct.h>

void getSoftwareUpdateConfiguration(
    int os_version,
    int* isAutomaticallyCheckForUpdatesManaged,
    int* isAutomaticallyCheckForUpdatesEnabled,
    int* isdoBackgroundDownloadManaged,
    int* doesBackgroundDownload,
    int* doesOSXAutoUpdatesManaged,
    int* doesOSXAutoUpdates,
    int* isAutomaticConfigDataCriticalUpdateInstallManaged,
    int* doesAutomaticConfigDataInstall,
    int* doesAutomaticCriticalUpdateInstall,
    int* lastCheckTimestamp) {
  // Starting with MacOS 15 (build version 24), the OSUpdate framework is used
  // over the SoftwareUpdate framework.
  _Bool os_framework = os_version >= 24;

  NSBundle* suBundle;
  suBundle = [NSBundle
      bundleWithPath:
          @"/System/Library/PrivateFrameworks/SoftwareUpdate.framework"];
  [suBundle load];

  Class SUSharedPrefs = [suBundle classNamed:@"SUSharedPrefs"];
  id manager = [SUSharedPrefs sharedPrefManager];

  NSBundle* osBundle;
  osBundle = [NSBundle
      bundleWithPath:@"/System/Library/PrivateFrameworks/OSUpdate.framework"];
  [osBundle load];

  Class SUOSUShimController = [osBundle classNamed:@"SUOSUShimController"];
  id settings = [SUOSUShimController alloc];

  _Bool value;

  value = os_framework
              ? [settings isAutomaticallyCheckForUpdatesPreferenceManaged]
              : [manager isAutomaticallyCheckForUpdatesManaged];
  *isAutomaticallyCheckForUpdatesManaged = value ? 1 : 0;

  value = os_framework
              ? [settings isAutomaticallyCheckForUpdatesPreferenceEnabled]
              : [manager isAutomaticallyCheckForUpdatesEnabled];
  *isAutomaticallyCheckForUpdatesEnabled = value ? 1 : 0;

  value = os_framework
              ? [settings isAutomaticallyDownloadUpdatesPreferenceManaged]
              : [manager isdoBackgroundDownloadManaged];
  *isdoBackgroundDownloadManaged = value ? 1 : 0;

  value = os_framework
              ? [settings isAutomaticallyDownloadUpdatesPreferenceEnabled]
              : [manager doesBackgroundDownload];
  *doesBackgroundDownload = value ? 1 : 0;

  value = os_framework
              ? [settings isAutomaticallyInstallMacOSUpdatesPreferenceManaged]
              : [manager isMacOSAutoUpdateManaged];
  *doesOSXAutoUpdatesManaged = value ? 1 : 0;

  if (os_framework) {
    value = [settings isAutomaticallyInstallMacOSUpdatesPreferenceEnabled];
    // Starting with MacOS 10.14 (build version 18), the method changed.
  } else if (os_version < 18) {
    value = [manager doesOSXAutoUpdates];
  } else {
    value = [manager doesMacOSAutoUpdate];
  }
  *doesOSXAutoUpdates = value ? 1 : 0;

  value =
      os_framework
          ? [settings
                isAutomaticallyInstallSecurityAndConfigUpdatesPreferenceManaged]
          : [manager isAutomaticConfigDataCriticalUpdateInstallManaged];
  *isAutomaticConfigDataCriticalUpdateInstallManaged = value ? 1 : 0;

  value =
      os_framework
          ? [settings
                isAutomaticallyInstallSecurityAndConfigUpdatesPreferenceEnabled]
          : [manager doesAutomaticConfigDataInstall];
  *doesAutomaticConfigDataInstall = value ? 1 : 0;

  value =
      os_framework
          ? [settings
                isAutomaticallyInstallSecurityAndConfigUpdatesPreferenceEnabled]
          : [manager doesAutomaticCriticalUpdateInstall];
  *doesAutomaticCriticalUpdateInstall = value ? 1 : 0;

  NSDate* lastCheckSuccessfulDate =
      os_framework ? (NSDate*)[settings latestSuccessfulScanDate]
                   : (NSDate*)[manager lastCheckSuccessfulDate];
  *lastCheckTimestamp = [lastCheckSuccessfulDate timeIntervalSince1970];

  [settings dealloc];

  return;
}

void getRecommendedUpdates() {
  NSBundle* bundle;
  bundle = [NSBundle
      bundleWithPath:
          @"/System/Library/PrivateFrameworks/SoftwareUpdate.framework"];
  [bundle load];

  Class SUSharedPrefs = [bundle classNamed:@"SUSharedPrefs"];
  id manager = [SUSharedPrefs sharedPrefManager];

  NSArray* updates = [manager recommendedUpdates];
  unsigned int i = 0;

  updatesFound([updates count]);

  for (id update in updates) {
    for (NSString* key in update) {
      NSString* value = [update objectForKey:key];
      updateKeyValueFound(
          i, (char*)[key UTF8String], (char*)[[value description] UTF8String]);
    }
    ++i;
  }

  return;
}
