package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/kolide/launcher/tools/packaging"
)

const (
	packageRoot = "tools/packaging/macos/root"
	scriptsRoot = "tools/packaging/macos/scripts"
)

func createMacPackage(pemKey []byte, id int, p packageParams) {
	// make temp directory for package
	pkgroot := filepath.Join(os.TempDir(), "pkgroot")
	os.RemoveAll(pkgroot)

	if err := copyDir(packageRoot, pkgroot); err != nil {
		log.Fatal(err)
	}
	etcPath := filepath.Join(pkgroot, "etc", "kolide")
	if err := os.MkdirAll(etcPath, dirMode); err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(pkgroot, "var", "kolide"), dirMode); err != nil {
		log.Fatal(err)
	}

	binPath := filepath.Join(pkgroot, "usr", "local", "kolide", "bin")
	if err := os.MkdirAll(binPath, dirMode); err != nil {
		log.Fatal(err)
	}

	t := tennant{id: id}
	secretPath := filepath.Join(etcPath, "token")
	token, err := t.Secret(pemKey)
	if err != nil {
		log.Fatal(err)
	}
	if err := ioutil.WriteFile(secretPath, []byte(token), 0400); err != nil {
		log.Fatal(err)
	}

	if err := copyDir(p.bindir, binPath); err != nil {
		log.Fatal(err)
	}

	if err := pkgbuild(pkgroot, "1.0.0", fmt.Sprintf("%s-launcher.pkg", t.Name())); err != nil {
		log.Fatal(err)
	}

	os.RemoveAll(pkgroot)

}

/*
runs the following pkgbuild command:
	pkgbuild \
	--root root \
	--scripts scripts \
	--identifier ${PKGID} \
	--version ${PKGVERSION} \
	out/${PKGNAME}-${PKGVERSION}.pkg
*/
func pkgbuild(pkgroot, version, pkgname string) error {
	identifier := "com.kolide.osquery"
	cmd := exec.Command("pkgbuild",
		"--root", pkgroot,
		"--scripts", scriptsRoot,
		"--identifier", identifier,
		"--version", version,
		fmt.Sprintf("build/%s", pkgname),
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

const (
	dirMode  = 0755
	fileMode = 0644
)

func copyDir(src, dest string) error {
	dir, err := os.Open(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, dirMode); err != nil {
		return err
	}

	files, err := dir.Readdir(-1)
	if err != nil {
		return err
	}
	for _, file := range files {
		srcptr := filepath.Join(src, file.Name())
		dstptr := filepath.Join(dest, file.Name())
		if file.IsDir() {
			if err := copyDir(srcptr, dstptr); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcptr, dstptr); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destfile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destfile.Close()

	_, err = io.Copy(destfile, source)
	if err != nil {
		return err
	}
	sourceinfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dest, sourceinfo.Mode())
}

func printTennant(pemKey []byte, id int, p packageParams) {
	t := tennant{id: id}
	token, err := t.Secret(pemKey)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Name: %s\n", t.Name())
	fmt.Printf("JWT: %s\n", token)
}

type tennant struct {
	id     int
	name   string
	secret string
}

type packageParams struct {
	bindir string //path to agent, osquery and other binaries that need to be in the pkg
}

func (t *tennant) Name() string {
	if t.name != "" {
		return t.name
	}
	return packaging.Munemo(t.id)
}

func (t *tennant) Secret(pemKey []byte) (token string, err error) {
	fingerPrint := fmt.Sprintf("% x", md5.Sum([]byte(pemKey)))
	fingerPrint = strings.Replace(fingerPrint, " ", ":", 15)

	var claims = struct {
		Tennant string `json:"tennant"`
		KID     string `json:"kid"`
		jwt.StandardClaims
	}{
		Tennant: t.Name(),
		KID:     fingerPrint,
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemKey)
	if err != nil {
		return "", fmt.Errorf("parsing pem key: %s", err)
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, &claims)
	signed, err := jwtToken.SignedString(key)
	if err != nil {
		return "", err
	}
	return signed, nil
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

	p := packageParams{bindir: *bindirPath}
	if printMode {
		printTennant(keyData, *flTennantID, p)
		return
	}

	if packageMode {
		createMacPackage(keyData, *flTennantID, p)
	}

}
