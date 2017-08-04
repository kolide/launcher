// +build ignore

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/kolide/updater/tuf"
)

func main() {
	var (
		flBinary   = flag.String("binary", "osquery", "which binary to use for assets")
		flInsecure = flag.Bool("insecure", false, "use insecure http client")
		flNotary   = flag.String("notary", "https://notary.kolide.com", "notary server url")
	)
	flag.Parse()

	gun := path.Join("kolide", *flBinary)
	localRepo := filepath.Join("autoupdate", "assets", fmt.Sprintf("%s-tuf", *flBinary))
	settingsOsquery := &tuf.Settings{
		LocalRepoPath: localRepo,
		NotaryURL:     *flNotary,
		GUN:           gun,
	}

	client := http.DefaultClient
	if *flInsecure {
		client = insecureClient()
	}

	if err := os.MkdirAll(settingsOsquery.LocalRepoPath, 0755); err != nil {
		log.Fatal(err)
	}

	if *flNotary != "" {
		bootstrapFromNotary(settingsOsquery, client)
	}

}

func bootstrapFromNotary(settings *tuf.Settings, client *http.Client) {
	roles := []string{"root.json", "snapshot.json", "timestamp.json", "targets.json"}
	for _, role := range roles {
		urlstring := settings.NotaryURL + path.Join("/v2/", settings.GUN, "_trust/tuf", role)
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
		roleFile := filepath.Join(settings.LocalRepoPath, role)
		if err := ioutil.WriteFile(roleFile, data, 0644); err != nil {
			log.Fatalf("could not save %s to %s: %s\n", urlstring, settings.LocalRepoPath, err)
		}
		fmt.Printf("saved %s to %s\n", urlstring, roleFile)
	}
}

func insecureClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}
