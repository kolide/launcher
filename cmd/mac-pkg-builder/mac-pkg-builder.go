package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/kolide/launcher/tools/packaging"
)

const (
	packageRoot = "tools/packaging/macos/root"
	scriptsRoot = "tools/packaging/macos/scripts"
)

func createMacPackage(pemKey []byte, id int, osqueryPath string) {
	// make temp directory for package
	pkgroot := filepath.Join(os.TempDir(), "pkgroot")
	os.RemoveAll(pkgroot)

	if err := packaging.CopyDir(packageRoot, pkgroot); err != nil {
		log.Fatal(err)
	}
	etcPath := filepath.Join(pkgroot, "etc", "kolide")
	if err := os.MkdirAll(etcPath, packaging.DirMode); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(pkgroot, "var", "kolide"), packaging.DirMode); err != nil {
		log.Fatal(err)
	}

	binPath := filepath.Join(pkgroot, "usr", "local", "kolide", "bin")
	if err := os.MkdirAll(binPath, packaging.DirMode); err != nil {
		log.Fatal(err)
	}

	secretPath := filepath.Join(etcPath, "token")
	tenantName := packaging.Munemo(id)
	token, err := packaging.Secret(tenantName, pemKey)
	if err != nil {
		log.Fatal(err)
	}
	if err := ioutil.WriteFile(secretPath, []byte(token), 0400); err != nil {
		log.Fatal(err)
	}

	if err := packaging.CopyDir(osqueryPath, binPath); err != nil {
		log.Fatal(err)
	}

	if err := packaging.Pkgbuild(pkgroot, scriptsRoot, "1.0.0", fmt.Sprintf("%s-launcher.pkg", tenantName)); err != nil {
		log.Fatal(err)
	}

	os.RemoveAll(pkgroot)
}

func main() {
	var (
		flKey       = flag.String("key", "", "path to rsa private key")
		flTennantID = flag.Int("id", 100001, "tennant id. must be greater than 1")
		flPrint     = flag.Bool("print", false, "print info for stdout -- requires passing a tennant id")
		flPackage   = flag.Bool("package", false, "generate macOS package")
		bindirPath  = flag.String("bindir", "./bin", "path to binaries")
	)
	flag.Parse()

	printMode := (*flTennantID > 0) && *flPrint
	packageMode := !printMode && *flPackage
	badInput := (*flTennantID == 0 && *flPrint) || *flKey == "" || (!printMode && !packageMode)
	if badInput {
		flag.Usage()
		os.Exit(1)
	}

	keyData, err := ioutil.ReadFile(*flKey)
	if err != nil {
		log.Fatal(err)
	}

	if printMode {
		name := packaging.Munemo(*flTennantID)
		token, err := packaging.Secret(name, keyData)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Name: %s\n", name)
		fmt.Printf("JWT: %s\n", token)

		return
	}

	if packageMode {
		createMacPackage(keyData, *flTennantID, *bindirPath)
	}
}
