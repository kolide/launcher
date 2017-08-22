package osquery

import (
	"context"
	"fmt"

	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const (
	sipEnabled                 = "sip_enabled"
	gatekeeperEnabled          = "gatekeeper_enabled"
	filevaultEnabled           = "filevault_enabled"
	firewallEnabled            = "firewall_enabled"
	screensaverPasswordEnabled = "screensaver_password_enabled"
)

func BestPractices(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn(sipEnabled),
		table.IntegerColumn(gatekeeperEnabled),
		table.IntegerColumn(filevaultEnabled),
		table.IntegerColumn(firewallEnabled),
		table.IntegerColumn(screensaverPasswordEnabled),
	}
	return table.NewPlugin("kolide_best_practices", columns, generateBestPractices(client))
}

func generateBestPractices(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		res := map[string]string{}

		row, err := client.QueryRow("SELECT enabled AS enabled FROM sip_config WHERE config_flag='sip'")
		if err != nil {
			return nil, errors.Wrap(err, "query sip_config")
		}
		res[sipEnabled] = row["enabled"]

		row, err = client.QueryRow("SELECT assessments_enabled AS enabled FROM gatekeeper")
		if err != nil {
			return nil, errors.Wrap(err, "query gatekeeper")
		}
		res[gatekeeperEnabled] = row["enabled"]

		row, err = client.QueryRow("SELECT de.encrypted AS enabled FROM mounts m join disk_encryption de ON m.device_alias = de.name WHERE m.path = '/'")
		if err != nil {
			return nil, errors.Wrap(err, "query filevault")
		}
		res[filevaultEnabled] = row["enabled"]

		row, err = client.QueryRow("SELECT global_state AS enabled FROM alf")
		if err != nil {
			return nil, errors.Wrap(err, "query firewall")
		}
		res[firewallEnabled] = row["enabled"]

		// TODO account for possibility of multiple logged in users
		row, err = client.QueryRow("SELECT value AS enabled FROM preferences WHERE domain='com.apple.screensaver' AND key='askForPassword' AND username in (SELECT user FROM logged_in_users) LIMIT 1")
		if err != nil {
			return nil, errors.Wrap(err, "query screensaver password")
		}
		res[screensaverPasswordEnabled] = row["enabled"]

		fmt.Println(res) // TODO remove before merge
		return []map[string]string{res}, nil
	}
}
