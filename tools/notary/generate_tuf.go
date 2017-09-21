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
	"time"

	"github.com/docker/notary/client"
	"github.com/docker/notary/trustpinning"
	"github.com/docker/notary/tuf/data"
	"github.com/kolide/launcher/tools/packaging"
)

func main() {
	var (
		flBinary          = flag.String("binary", "osqueryd", "which binary to use for assets")
		flNotaryConfigDir = flag.String("notary_config_dir", filepath.Join(packaging.LauncherSource(), "tools/notary/config"), "notary base directory")
	)
	flag.Parse()

	gun := path.Join("kolide", *flBinary)
	localRepo := filepath.Join("autoupdate", "assets", fmt.Sprintf("%s-tuf", *flBinary))

	if err := os.MkdirAll(localRepo, 0755); err != nil {
		log.Fatal(err)
	}

	bootstrapFromNotary(*flNotaryConfigDir, localRepo, gun)
}

func bootstrapFromNotary(baseDir, localRepo, gun string) {
	// Read Notary configuration
	fin, err := os.Open(filepath.Join(baseDir, "config.json"))
	if err != nil {
		log.Fatal(err)
	}
	defer fin.Close()

	// Decode the Notary configuration into a struct
	conf := struct {
		RemoteServer RemoteServer `json:"remote_server"`
	}{}
	if err = json.NewDecoder(fin).Decode(&conf); err != nil {
		log.Fatal(err)
	}

	// Create the transport
	transport, err := getTransport(baseDir, conf.RemoteServer)
	if err != nil {
		log.Fatal(err)
	}

	// Safely fetch and validate all TUF metadata from remote Notary server.
	repo, err := client.NewFileCachedRepository(
		baseDir,
		data.GUN(gun),
		conf.RemoteServer.URL,
		transport,
		passwordRetriever,
		trustpinning.TrustPinConfig{},
	)
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
