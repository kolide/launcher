package main

import (
	"bytes"
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
)

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

const (
	packageRoot = "mac-pkg/root"
	scriptsRoot = "mac-pkg/scripts"
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
	if err := ioutil.WriteFile(secretPath, []byte(token), fileMode); err != nil {
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
	pkgbuild --root root --scripts scripts --identifier ${PKGID} --version ${PKGVERSION} out/${PKGNAME}-${PKGVERSION}.pkg
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

const dirMode = 0755
const fileMode = 0644

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
	m := newMunemo()
	m.tos(t.id)
	t.name = m.String()
	return t.name
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

// munemo is based off of the ruby library https://github.com/jmettraux/munemo
// it provides a deterministic way to create a tennant name from UID
type munemo struct {
	count int
	neg   string
	syls  []string
	w     *bytes.Buffer // string buffer
}

func newMunemo() *munemo {
	m := &munemo{
		syls: []string{"ba", "bi", "bu", "be", "bo", "cha", "chi", "chu", "che", "cho", "da", "di", "du", "de", "do", "fa", "fi", "fu", "fe", "fo", "ga", "gi", "gu", "ge", "go", "ha", "hi", "hu", "he", "ho", "ja", "ji", "ju", "je", "jo", "ka", "ki", "ku", "ke", "ko", "la", "li", "lu", "le", "lo", "ma", "mi", "mu", "me", "mo", "na", "ni", "nu", "ne", "no", "pa", "pi", "pu", "pe", "po", "ra", "ri", "ru", "re", "ro", "sa", "si", "su", "se", "so", "sha", "shi", "shu", "she", "sho", "ta", "ti", "tu", "te", "to", "tsa", "tsi", "tsu", "tse", "tso", "wa", "wi", "wu", "we", "wo", "ya", "yi", "yu", "ye", "yo", "za", "zi", "zu", "ze", "zo"},
		neg:  "xa",
		w:    new(bytes.Buffer),
	}
	m.count = len(m.syls)
	return m
}

func (m *munemo) String() string {
	return m.w.String()
}

func (m *munemo) tos(num int) {
	if num < 0 {
		m.w.Write([]byte(m.neg))
		return
	}

	mod := num % m.count
	rst := num / m.count

	if rst > 0 {
		m.tos(rst)
	}

	m.w.Write([]byte(m.syls[mod]))
}
