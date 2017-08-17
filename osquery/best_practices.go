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

func getQueryRows(client *osquery.ExtensionManagerClient, query string) ([]map[string]string, error) {
	res, err := client.Query(query)
	if err != nil {
		return nil, errors.Wrap(err, "transport error in query")
	}
	if res.Status == nil {
		return nil, errors.New("query returned nil status")
	}
	if res.Status.Code != 0 {
		return nil, errors.Errorf("query returned error: %s", res.Status.Message)
	}
	return res.Response, nil
}

func getQueryRow(client *osquery.ExtensionManagerClient, query string) (map[string]string, error) {
	res, err := getQueryRows(client, query)
	if err != nil {
		return nil, err
	}
	if len(res) != 1 {
		return nil, errors.Errorf("expected 1 row, got %d", len(res))
	}
	return res[0], nil
}

func generateBestPractices(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		row := map[string]string{}

		res, err := getQueryRow(client, "select enabled as enabled from sip_config where config_flag='sip'")
		if err != nil {
			return nil, errors.Wrap(err, "query sip_config")
		}
		row[sipEnabled] = res["enabled"]

		res, err = getQueryRow(client, "select assessments_enabled as enabled from gatekeeper")
		if err != nil {
			return nil, errors.Wrap(err, "query gatekeeper")
		}
		row[gatekeeperEnabled] = res["enabled"]

		res, err = getQueryRow(client, "select de.encrypted as enabled from mounts m join disk_encryption de on m.device_alias = de.name where m.path = '/'")
		if err != nil {
			return nil, errors.Wrap(err, "query filevault")
		}
		row[filevaultEnabled] = res["enabled"]

		fmt.Println(row) // TODO remove before merge
		return []map[string]string{row}, nil
	}
}
