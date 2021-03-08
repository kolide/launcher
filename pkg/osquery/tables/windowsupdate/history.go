// +build windows

package windowsupdate

import (
	"context"
	"strconv"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
	"github.com/scjalliance/comshim"
)

func HistoryTablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		table.TextColumn("locale"),
		table.IntegerColumn("is_default"),
	)

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_windows_update_history", columns, t.generateHistory)
}

func (t *Table) generateHistory(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	for _, locale := range tablehelpers.GetConstraints(queryContext, "locale", tablehelpers.WithDefaults("_default")) {
		result, err := t.historyLocale(locale, queryContext)
		if err != nil {
			level.Info(t.logger).Log("msg", "got error enumerating history", "locale", locale, "err", err)
			continue
		}
		results = append(results, result...)

	}

	return results, nil
}

func (t *Table) historyLocale(locale string, queryContext table.QueryContext) ([]map[string]string, error) {
	comshim.Add(1)
	defer comshim.Done()

	var results []map[string]string

	session, setLocale, is_default, err := getSession(locale)
	if err != nil {
		return nil, errors.Wrap(err, "new session")
	}

	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return nil, errors.Wrap(err, "new searcher")
	}

	searchResults, err := searcher.QueryHistoryAll()
	if err != nil {
		return nil, errors.Wrap(err, "get all history")
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flatData, err := t.flattenOutput(dataQuery, searchResults)
		if err != nil {
			level.Info(t.logger).Log("msg", "flatten failed", "err", err)
			continue
		}

		rowData := map[string]string{
			"locale":     setLocale,
			"is_default": strconv.Itoa(is_default),
		}

		results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
	}

	return results, nil

}
