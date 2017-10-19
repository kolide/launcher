package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kolide/kit/env"
	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/tools/packaging"
)

func main() {
	var (
		flVersion  = flag.String("version", env.String("VERSION", "stable"), "the osqueryd version to download")
		flPlatform = flag.String("platform", env.String("PLATFORM", ""), "the platform to download osqueryd for (ie: darwin, linux)")
		flOutput   = flag.String("output", env.String("OUTPUT", ""), "the path where the binary should be output")
	)
	flag.Parse()

	if *flPlatform == "" {
		fmt.Println("The --platform option must be defined")
		os.Exit(1)
	}

	path, err := packaging.FetchOsquerydBinary(*flVersion, *flPlatform)
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
