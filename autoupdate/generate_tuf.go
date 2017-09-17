// +build ignore

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/notary/client"
	"github.com/docker/notary/trustpinning"
	"github.com/docker/notary/tuf/data"
	"github.com/kolide/updater/tuf"
)

func main() {
	var (
		flBinary      = flag.String("binary", "osqueryd", "which binary to use for assets")
		flInsecure    = flag.Bool("insecure", false, "use insecure http client")
		flBaseDir     = flag.String("base-dir", filepath.Join(os.Getenv("HOME"), ".notary"), "notary base directory")
		flUnvalidated = flag.Bool("unvalidated", false, "does not use notary to validate tuf repo, staging/testing only")
		flURL         = flag.String("url", "https://notary.kolide.com", "URL to use with unvalidated option")
	)
	flag.Parse()

	gun := path.Join("kolide", *flBinary)
	localRepo := filepath.Join("autoupdate", "assets", fmt.Sprintf("%s-tuf", *flBinary))

	if err := os.MkdirAll(localRepo, 0755); err != nil {
		log.Fatal(err)
	}

	if *flUnvalidated {
		bootstrapUnvalidated(*flURL, localRepo, gun, *flInsecure)
		return
	}
	if *flInsecure {
		log.Fatal("tls insecure can't be true if using notary to validate tuf")
	}

	bootstrapFromNotary(*flBaseDir, localRepo, gun)

}

func bootstrapUnvalidated(url, localRepo, gun string, insecure bool) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	roles := []string{"root.json", "snapshot.json", "timestamp.json"}
	for _, role := range roles {
		urlstring := url + path.Join("/v2/", gun, "_trust/tuf", role)
		resp, err := client.Get(urlstring)
		if err != nil {
			log.Fatalf("could not download %s: %s\n", urlstring, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("got unexpected http Status %s for %s", resp.Status, urlstring)
		}
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		roleFile := filepath.Join(localRepo, role)
		if err := ioutil.WriteFile(roleFile, data, 0644); err != nil {
			log.Fatalf("could not save %s to %s: %s\n", urlstring, localRepo, err)
		}
		fmt.Printf("saved %s to %s\n", urlstring, roleFile)
	}

	// We need to recursively decode delegates (target is a special case) and handle each
	// child delegate
	err := retrieveDelegate(
		url+path.Join("/v2/", gun, "_trust/tuf"),
		localRepo,
		"targets.json",
		client,
	)
	if err != nil {
		log.Fatal(err)
	}
}

func retrieveDelegate(rootURL, rootLocalRepo, role string, client *http.Client) error {
	// Download the role, write it's content to file  and decode it so we
	// can easily determine if it has delegates of it's own.
	url := fmt.Sprintf("%s/%s", rootURL, role)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var delegate tuf.Targets
	if err = json.Unmarshal(data, &delegate); err != nil {
		return err
	}
	rolePath := filepath.Join(rootLocalRepo, role)
	if err = ioutil.WriteFile(rolePath, data, 0644); err != nil {
		return err
	}
	fmt.Printf("saved %s to %s\n", url, rolePath)
	if len(delegate.Signed.Delegations.Roles) == 0 {
		return nil
	}
	// If delegate has children get those and build directory structure
	// The role name is the name of the directory containing all it's children
	roleName := strings.Replace(role, ".json", "", 1)
	childrenDir := filepath.Join(rootLocalRepo, roleName)
	if err = os.MkdirAll(childrenDir, 0755); err != nil {
		return err
	}
	childrenRootURL := fmt.Sprintf("%s/%s", rootURL, roleName)
	for _, del := range delegate.Signed.Delegations.Roles {
		childRole := path.Base(del.Name) + ".json"
		if err = retrieveDelegate(childrenRootURL, childrenDir, childRole, client); err != nil {
			return err
		}
	}

	return nil
}

func bootstrapFromNotary(baseDir, localRepo, gun string) {
	// Read Notary configuration
	fin, err := os.Open(filepath.Join(baseDir, "config.json"))
	if err != nil {
		log.Fatal(err)
	}
	defer fin.Close()
	conf := struct {
		RemoteServer RemoteServer `json:"remote_server"`
	}{}
	if err = json.NewDecoder(fin).Decode(&conf); err != nil {
		log.Fatal(err)
	}
	rt, err := getTransport(baseDir, conf.RemoteServer)
	if err != nil {
		log.Fatal(err)
	}
	// Safely fetch and validate all TUF metadata from remote Notary server.
	repo, err := client.NewFileCachedRepository(baseDir, data.GUN(gun), conf.RemoteServer.URL, rt, passwordRetriever, trustpinning.TrustPinConfig{})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := repo.GetAllTargetMetadataByName(""); err != nil {
		log.Fatal(err)
	}
	// Stage TUF metadata and create bindata from it so it can be distributed as part of the Launcher executable
	source := filepath.Join(baseDir, "tuf", gun, "metadata")
	if err := CopyDir(source, localRepo); err != nil {
		log.Fatal(err)
	}
	log.Printf("successfully bootstrapped and validated TUF repo %q\n", gun)
}

type RemoteServer struct {
	URL        string `json:"url"`
	RootCA     string `json:"root_ca"`
	ClientCert string `json:"tls_client_cert"`
	ClientKey  string `json:"tls_client_key"`
}

func getTransport(baseDir string, svr RemoteServer) (http.RoundTripper, error) {
	rootCA := filepath.Join(baseDir, svr.RootCA)
	var tlsConfig tls.Config
	if svr.RootCA != "" {
		pool := x509.NewCertPool()
		pem, err := ioutil.ReadFile(rootCA)
		if err != nil {
			return nil, err
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("could not append to ca cert pool %q", rootCA)
		}
		tlsConfig.RootCAs = pool
	}
	if svr.ClientKey != "" && svr.ClientKey != "" {
		clientCert := filepath.Join(baseDir, svr.ClientCert)
		clientKey := filepath.Join(baseDir, svr.ClientKey)

		tlsCert, err := tls.LoadX509KeyPair(clientCert, clientKey)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{tlsCert}
	}
	rt := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
			DualStack: true,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig:     &tlsConfig,
		DisableKeepAlives:   true,
	}

	return rt, nil
}

func passwordRetriever(key, alias string, createNew bool, attempts int) (pass string, giveUp bool, err error) {
	pass = os.Getenv(key)
	if pass == "" {
		err = fmt.Errorf("Missing pass phrase env var %q", key)
	}
	return pass, giveUp, err
}

// Copied from github.com/kolide/launcher/tools/packaging/filesystem.go because if this is imported go run won't
// compile this as it builds autoupdater.go which needs bindata.go which we haven't created yet.
func CopyDir(src, dest string) error {
	dir, err := os.Open(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
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
			if err := CopyDir(srcptr, dstptr); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcptr, dstptr); err != nil {
				return err
			}
		}
	}
	return nil
}

// Copied from github.com/kolide/launcher/tools/packaging/filesystem.go because if packaging is imported this won't
// compile.  It references autoupdater.go which needs bindata.go which we haven't created yet.
func CopyFile(src, dest string) error {
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
