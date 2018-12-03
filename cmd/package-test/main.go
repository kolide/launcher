package main

import (
	"os"

	"github.com/kolide/launcher/pkg/packagingTNG/deb"
	"github.com/kolide/launcher/pkg/packagingTNG/pkg"
	"github.com/kolide/launcher/pkg/packagingTNG/rpm"
)

func main() {

	packageRoot := "/Users/seph/go/src/github.com/kolide/launcher/pkg/packagingTNG"

	rpmOut, err := os.Create("/tmp/test.rpm")
	if err != nil {
		panic(err)
	}
	if err := rpm.Package(rpmOut, "test", packageRoot, rpm.WithVersion("0.0.1")); err != nil {
		panic(err)
	}

	debOut, err := os.Create("/tmp/test.deb")
	if err != nil {
		panic(err)
	}
	if err := deb.Package(debOut, "test", packageRoot, deb.WithVersion("0.0.1")); err != nil {
		panic(err)
	}

	pkgOut, err := os.Create("/tmp/test.pkg")
	if err != nil {
		panic(err)
	}
	if err := pkg.Package(pkgOut, "test", packageRoot, pkg.WithVersion("0.0.1"),
		pkg.WithSigningKey("Developer ID Installer: Kolide Inc (YZ3EM74M78)")); err != nil {
		panic(err)
	}

	// Examine outputs:
	//
	// cat /tmp/test.rpm | cpio -idmv
	// ar -p /tmp/test.deb  data.tar.gz | tar tzf -  # NOTE: does not work on osx?
	// tar tzf /tmp/test.pkg

}
