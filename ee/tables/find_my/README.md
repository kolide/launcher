# Find My

## Generating header files

Header files generated via ipsw (`brew install blacktop/tap/ipsw`):

```
ipsw class-dump /System/Volumes/Preboot/Cryptexes/OS/System/Library/dyld/dyld_shared_cache_arm64e FindMyDevice --headers
```

Note that the `-fmodules` is required in CFLAGS to permit the `@import` statements in the generated header files.
