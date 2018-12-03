package main

import (
	"fmt"
	"os"

	"github.com/kolide/launcher/pkg/packagekit"
)

func main() {

	packageRoot := "/Users/seph/go/src/github.com/kolide/launcher/pkg"

	po := &packagekit.PackageOptions{
		Name:    "test-empty",
		Version: "0.0.0",
		Root:    packageRoot,
	}

	fmt.Println("Starting RPM")
	rpmOut, err := os.Create("/tmp/test.rpm")
	if err != nil {
		panic(err)
	}
	if err := packagekit.PackageRPM(rpmOut, po); err != nil {
		panic(err)
	}

	fmt.Println("Starting deb")
	debOut, err := os.Create("/tmp/test.deb")
	if err != nil {
		panic(err)
	}
	if err := packagekit.PackageDeb(debOut, po); err != nil {
		panic(err)
	}

	fmt.Println("Starting pkg")
	pkgOut, err := os.Create("/tmp/test.pkg")
	if err != nil {
		panic(err)
	}
	if err := packagekit.PackagePkg(pkgOut, po, packagekit.WithSigningKey("Developer ID Installer: Kolide Inc (YZ3EM74M78)")); err != nil {
		panic(err)
	}

	// Examine outputs:
	//
	// cat /tmp/test.rpm | cpio -idmv
	// ar -p /tmp/test.deb  data.tar.gz | tar tzf -  # NOTE: does not work on osx?
	// tar tzf /tmp/test.pkg

}
