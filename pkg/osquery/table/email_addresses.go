package table

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func EmailAddresses(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("email"),
		table.TextColumn("domain"),
	}
	t := &emailAddressesTable{
		onePasswordAccountConfig: &onePasswordAccountConfig{client: client},
	}
	return table.NewPlugin("kolide_email_addresses", columns, t.generateEmailAddresses)
}

type emailAddressesTable struct {
	onePasswordAccountConfig *onePasswordAccountConfig
}

func (t *emailAddressesTable) generateEmailAddresses(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	// add results from chrome profiles
	chromeResults, err := generateEmailAddressesFromChromeState()
	if err != nil {
		return nil, errors.Wrap(err, "get email addresses from chrome state")
	}
	results = append(results, chromeResults...)

	// add results from 1password
	onePassResults, err := t.onePasswordAccountConfig.generate(ctx, queryContext)
	if err != nil {
		return nil, errors.Wrap(err, "adding email results from 1password config")
	}
	results = append(results, onePassResults...)

	return results, nil
}

func generateEmailAddressesFromChromeState() ([]map[string]string, error) {
	var results []map[string]string
	for _, stateFilePath := range findChromeStateFiles() {
		fileContent, err := ioutil.ReadFile(stateFilePath)
		if os.IsNotExist(err) {
			break
		}
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
	return results, nil
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
