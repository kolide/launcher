extern void updatesFound(unsigned int);
extern void updateKeyValueFound(unsigned int, char*, char*);
extern void productsFound(unsigned int);
extern void productKeyValueFound(unsigned int, char*, char*);
extern void productNestedKeyValueFound(unsigned int, char*, char*, char*);

void getSoftwareUpdateConfiguration(int os_version,
                                    int* isAutomaticallyCheckForUpdatesManaged,
                                    int* isAutomaticallyCheckForUpdatesEnabled,
                                    int* doesBackgroundDownload,
                                    int* doesAppStoreAutoUpdates,
                                    int* doesOSXAutoUpdates,
                                    int* doesAutomaticCriticalUpdateInstall,
                                    int* lastCheckTimestamp);

void getRecommendedUpdates();

void getAvailableProducts();