package main

import (
	"os"

	packageTNG "github.com/kolide/launcher/pkg/packagingTNG"
)

func main() {

	packageRoot := "/Users/seph/go/src/github.com/kolide/launcher/"

	po := &packageTNG.PackageOptions{
		Name:    "test-empty",
		Version: "0.0.0",
		Root:    packageRoot,
	}

	rpmOut, err := os.Create("/tmp/test.rpm")
	if err != nil {
		panic(err)
	}
	if err := packageTNG.PackageRPM(rpmOut, po); err != nil {
		panic(err)
	}

	debOut, err := os.Create("/tmp/test.deb")
	if err != nil {
		panic(err)
	}
	if err := packageTNG.PackageDeb(debOut, po); err != nil {
		panic(err)
	}

	pkgOut, err := os.Create("/tmp/test.pkg")
	if err != nil {
		panic(err)
	}
	if err := packageTNG.PackagePkg(pkgOut, po, packageTNG.WithSigningKey("Developer ID Installer: Kolide Inc (YZ3EM74M78)")); err != nil {
		panic(err)
	}

	// Examine outputs:
	//
	// cat /tmp/test.rpm | cpio -idmv
	// ar -p /tmp/test.deb  data.tar.gz | tar tzf -  # NOTE: does not work on osx?
	// tar tzf /tmp/test.pkg

}
