package osquery

import (
	"context"
	"fmt"

	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const (
	sipEnabled        = "sip_enabled"
	gatekeeperEnabled = "gatekeeper_enabled"
	filevaultEnabled  = "filevault_enabled"
)

func BestPractices(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.IntegerColumn(sipEnabled),
		table.IntegerColumn(gatekeeperEnabled),
		table.IntegerColumn(filevaultEnabled),
	}
	return table.NewPlugin("kolide_best_practices", columns, generateBestPractices(client))
}

func generateBestPractices(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		row := map[string]string{}

		res, err := client.QueryRow("SELECT enabled AS enabled FROM sip_config WHERE config_flag='sip'")
		if err != nil {
			return nil, errors.Wrap(err, "query sip_config")
		}
		row[sipEnabled] = res["enabled"]

		res, err = client.QueryRow("SELECT assessments_enabled AS enabled FROM gatekeeper")
		if err != nil {
			return nil, errors.Wrap(err, "query gatekeeper")
		}
		row[gatekeeperEnabled] = res["enabled"]

		res, err = client.QueryRow("SELECT de.encrypted AS enabled FROM mounts m join disk_encryption de ON m.device_alias = de.name WHERE m.path = '/'")
		if err != nil {
			return nil, errors.Wrap(err, "query filevault")
		}
		row[filevaultEnabled] = res["enabled"]

		fmt.Println(row) // TODO remove before merge
		return []map[string]string{row}, nil
	}
}
