package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/kolide/launcher/pkg/packaging"
)

func runListTargets(_args []string) error {
	platformFlavors := packaging.KnownPlatformFlavors()
	initFlavors := packaging.KnownInitFlavors()
	packageFlavors := packaging.KnownPackageFlavors()

	outFH := os.Stdout

	fmt.Fprintf(outFH, "Packaging Target Matrix\n")
	fmt.Fprintf(outFH, "Select one from each column, hyphen separated.\n")
	fmt.Fprintf(outFH, "Not all combinations make sense\n")
	fmt.Fprintf(outFH, "A common target: `darwin-launchd-pkg`\n")
	fmt.Fprintf(outFH, "\n")

	w := tabwriter.NewWriter(outFH, 0, 4, 4, ' ', 0)

	line := 0
	for {
		hasPlatform := line < len(platformFlavors)
		hasInit := line < len(initFlavors)
		hasPackage := line < len(packageFlavors)

		if !hasPlatform && !hasInit && !hasPackage {
			break
		}

		platformFlavor := ""
		if hasPlatform {
			platformFlavor = platformFlavors[line]
		}

		initFlavor := ""
		if hasInit {
			initFlavor = initFlavors[line]
		}

		packageFlavor := ""
		if hasPackage {
			packageFlavor = packageFlavors[line]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", platformFlavor, initFlavor, packageFlavor)

		line++
	}
	w.Flush()

	return nil
}
