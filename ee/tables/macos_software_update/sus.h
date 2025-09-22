// Go callbacks
extern void updatesFound(unsigned int);
extern void updateKeyValueFound(unsigned int, char*, char*);

// Gets software update config flags from SUSharedPrefs API
void getSoftwareUpdateConfiguration(
    int os_version,
    int* isAutomaticallyCheckForUpdatesManaged,
    int* isAutomaticallyCheckForUpdatesEnabled,
    int* isdoBackgroundDownloadManaged,
    int* doesBackgroundDownload,
    int* doesAppStoreAutoUpdates,
    int* doesOSXAutoUpdatesManaged,
    int* doesOSXAutoUpdates,
    int* isAutomaticConfigDataCriticalUpdateInstallManaged,
    int* doesAutomaticConfigDataInstall,
    int* doesAutomaticCriticalUpdateInstall,
    int* lastCheckTimestamp);

// Gets recommended updates from the SUSharedPrefs API
void getRecommendedUpdates();

// Gets the available products via the SUScanController API
void getAvailableProducts();