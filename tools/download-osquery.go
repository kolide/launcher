package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/packaging"
)

func main() {
	var (
		flVersion  = flag.String("version", env.String("VERSION", "stable"), "the osqueryd version to download")
		flPlatform = flag.String("platform", env.String("PLATFORM", ""), "the platform to download osqueryd for (ie: darwin, linux)")
		flOutput   = flag.String("output", env.String("OUTPUT", ""), "the path where the binary should be output")
		flCacheDir = flag.String("cache_dir", env.String("CACHE_DIR", ""), "Directory to cache downloads in (default: random)")
	)
	flag.Parse()

	if *flPlatform == "" {
		fmt.Println("The --platform option must be defined")
		os.Exit(1)
	}

	// If we have a cacheDir, use it. Otherwise. set something random.
	cacheDir := *flCacheDir
	var err error
	if *flCacheDir == "" {
		cacheDir, err = ioutil.TempDir("", "download_cache")
		if err != nil {
			fmt.Printf("Could not create temp dir for caching files %v", err)
			os.Exit(1)
		}
		defer os.RemoveAll(cacheDir)
	}

	ctx := context.Background()

	path, err := packaging.FetchBinary(ctx, cacheDir, "osqueryd", *flVersion, *flPlatform)
	if err != nil {
		fmt.Println("An error occurred fetching the osqueryd binary: ", err)
		os.Exit(1)
	}

	if *flOutput != "" {
		if err := fs.CopyFile(path, *flOutput); err != nil {
			fmt.Printf("Couldn't copy file to %s: %s", *flOutput, err)
			os.Exit(1)
		}
		fmt.Println(*flOutput)
	} else {
		fmt.Println(path)
	}
}
