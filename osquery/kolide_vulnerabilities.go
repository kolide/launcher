package osquery

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"

	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func KolideVulnerabilities(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("name"),
		table.IntegerColumn("vulnerable"),
		table.TextColumn("details"),
	}
	return table.NewPlugin("kolide_vulnerabilities", columns, generateKolideVulnerabilities(client))
}

var generateFuncs = []func() map[string]string{
	generateCVE_2017_7149,
}

func generateKolideVulnerabilities(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		results := []map[string]string{}

		for _, f := range generateFuncs {
			results = append(results, f())
		}

		return results, nil
	}
}

func generateCVE_2017_7149() map[string]string {
	row := map[string]string{"name": "CVE-2017-7149"}
	volumes, err := getEncryptedAPFSVolumes()
	if err != nil {
		return row
	}

	foo := struct {
		Vulnerable []string `json:"vulnerable"`
	}{}
	for _, vol := range volumes {
		if checkVolumeVulnerability(vol) {
			foo.Vulnerable = append(foo.Vulnerable, vol)
		}
	}

	if len(foo.Vulnerable) == 0 {
		row["vulnerable"] = "0"
		return row
	}

	row["vulnerable"] = "1"

	detailJSON, err := json.Marshal(foo)
	if err != nil {
		return row
	}
	row["details"] = string(detailJSON)

	return row
}

// getEncryptedAPFSVolumes returns the list of volume names that are encrypted
// APFS volumes.
func getEncryptedAPFSVolumes() ([]string, error) {
	cmd := exec.Command("diskutil", "apfs", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	volumeSection := regexp.MustCompile(`(?s)Volume .+? Encrypted:\s+(Yes|No)`)
	isEncrypted := regexp.MustCompile(`Encrypted:\s+Yes`)
	volumeName := regexp.MustCompile(`Volume (\S+)`)

	volumes := []string{}
	for _, section := range volumeSection.FindAllString(string(out), -1) {
		if !isEncrypted.MatchString(section) {
			// Not an encrypted volume
			continue
		}

		matches := volumeName.FindStringSubmatch(section)
		if len(matches) != 2 {
			continue
		}

		volumes = append(volumes, matches[1])
	}

	return volumes, nil
}

func checkVolumeVulnerability(volume string) bool {
	cmd := exec.Command("diskutil", "apfs", "listCryptoUsers", volume)
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	userSectionWithHint := regexp.MustCompile(`(?s) (\S+-\S+-\S+-\S+-\S+).+? Hint: ([^\n]+)`)
	for _, matches := range userSectionWithHint.FindAllStringSubmatch(string(out), -1) {
		if len(matches) != 3 {
			continue
		}
		uuid := matches[1]
		passHint := matches[2]

		if testVolumeUser(volume, uuid, passHint) {
			return true
		}
	}

	return false
}

func testVolumeUser(volume, uuid, passHint string) bool {
	cmd := exec.Command("diskutil", "apfs", "unlockVolume", volume, "-verify", "-user", uuid, "-passphrase", passHint)
	err := cmd.Run()
	// If cmd exits zero, the password hint was the password
	return err == nil
}
