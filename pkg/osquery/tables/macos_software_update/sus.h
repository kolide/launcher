extern void updatesFound(unsigned int);
extern void updateKeyValueFound(unsigned int, char*, char*);

void getSoftwareUpdateConfiguration(int os_version,
                                    int* isAutomaticallyCheckForUpdatesManaged,
                                    int* isAutomaticallyCheckForUpdatesEnabled,
                                    int* doesBackgroundDownload,
                                    int* doesAppStoreAutoUpdates,
                                    int* doesOSXAutoUpdates,
                                    int* doesAutomaticCriticalUpdateInstall,
                                    int* lastCheckTimestamp);

void getRecommendedUpdates();