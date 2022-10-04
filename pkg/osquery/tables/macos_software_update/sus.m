#include "sus.h"

#import <Cocoa/Cocoa.h>
#import <SUSharedPrefs.h>

void softwareUpdate(
	int os_version,
	int *isAutomaticallyCheckForUpdatesManaged,
	int *isAutomaticallyCheckForUpdatesEnabled,
	int *doesBackgroundDownload,
	int *doesAppStoreAutoUpdates,
	int *doesOSXAutoUpdates,
	int *doesAutomaticCriticalUpdateInstall,
	int *lastCheckTimestamp
) {
	NSBundle *bundle;
	bundle = [NSBundle bundleWithPath:@"/System/Library/PrivateFrameworks/SoftwareUpdate.framework"];
	[bundle load];

	Class SUSharedPrefs = [bundle classNamed:@"SUSharedPrefs"];
	id manager = [SUSharedPrefs sharedPrefManager];

	BOOL val = [manager isAutomaticallyCheckForUpdatesManaged];
	if (val) {
		*isAutomaticallyCheckForUpdatesManaged = 1;
	}

	val = [manager isAutomaticallyCheckForUpdatesEnabled];
	if (val) {
		*isAutomaticallyCheckForUpdatesEnabled = 1;
	}

	val = [manager doesBackgroundDownload];
	if (val) {
		*doesBackgroundDownload = 1;
	}

	val = [manager doesAppStoreAutoUpdates];
	if (val) {
		*doesAppStoreAutoUpdates = 1;
	}

	// before 10.13 (build ver 17) it's called doesMacOSAutoUpdate.
	if (os_version >= 18) {
		val = [manager doesMacOSAutoUpdate];
		if (val) {
			*doesOSXAutoUpdates = 1;
		}
	} else {
		val = [manager doesOSXAutoUpdates];
		if (val) {
			*doesOSXAutoUpdates = 1;
		}
	}

	val = [manager doesAutomaticCriticalUpdateInstall];
	if (val) {
		*doesAutomaticCriticalUpdateInstall = 1;
	}
	NSDate * lastCheckSuccessfulDate = (NSDate *)[manager lastCheckSuccessfulDate];
	*lastCheckTimestamp = [lastCheckSuccessfulDate timeIntervalSince1970];
	return;
}