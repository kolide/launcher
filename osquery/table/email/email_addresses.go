package email

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func Addresses(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("email"),
		table.TextColumn("domain"),
	}
	return table.NewPlugin("kolide_email_addresses", columns, generateEmailAddresses(client))
}

func generateEmailAddresses(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		results := []map[string]string{}

		switch runtime.GOOS {
		case "darwin":
			// Google Chrome State Files
			for _, stateFilePath := range findChromeStateFiles() {
				fileContent, err := ioutil.ReadFile(stateFilePath)
				if err != nil {
					return nil, errors.Wrapf(err, "could not read file %s", stateFilePath)
				}

				var parsedStateFileContent chromeLocalStateFile
				if err := json.Unmarshal(fileContent, &parsedStateFileContent); err != nil {
					return nil, errors.Wrap(err, "could not unmarshal json file")
				}

				for _, profile := range parsedStateFileContent.Profile.InfoCache {
					results = addEmailToResults(profile.Username, results)
				}
			}
		}

		return results, nil
	}
}

type chromeLocalStateFile struct {
	Profile struct {
		InfoCache map[string]struct {
			Username string `json:"user_name"`
		} `json:"info_cache"`
	} `json:"profile"`
}

func emailDomain(email string) string {
	parts := strings.Split(email, "@")
	switch len(parts) {
	case 0:
		return email
	default:
		return parts[len(parts)-1]
	}
}

func addEmailToResults(email string, results []map[string]string) []map[string]string {
	return append(results, map[string]string{
		"email":  email,
		"domain": emailDomain(email),
	})
}

func findChromeStateFiles() []string {
	chromeLocalStateFiles := []string{}
	filesInUser, err := ioutil.ReadDir("/Users")
	if err != nil {
		return []string{}
	}
	for _, f := range filesInUser {
		if f.IsDir() && (f.Name() != "Guest" || f.Name() != "Shared") {
			stateFilePath := filepath.Join("/Users", f.Name(), "Library/Application Support/Google/Chrome/Local State")
			if _, err := os.Stat(stateFilePath); err == nil {
				chromeLocalStateFiles = append(chromeLocalStateFiles, stateFilePath)
			}
		}
	}

	return chromeLocalStateFiles
}
