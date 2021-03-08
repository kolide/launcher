// +build windows

package windowsupdate

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/dataflattentable"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/windows/windowsupdate"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
	"github.com/scjalliance/comshim"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		table.TextColumn("locale"),
		table.IntegerColumn("is_default"),
	)

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_windows_updates", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	comshim.Add(1)
	defer comshim.Done()

	var results []map[string]string

	for _, locale := range tablehelpers.GetConstraints(queryContext, "locale", tablehelpers.WithDefaults("_default")) {
		result, err := t.searchLocale(locale, queryContext)
		if err != nil {
			level.Info(t.logger).Log("msg", "got error searching for updates", "locale", locale, "err", err)
			continue
		}
		results = append(results, result...)

	}

	return results, nil
}

func (t *Table) searchLocale(locale string, queryContext table.QueryContext) ([]map[string]string, error) {
	level.Debug(t.logger).Log("msg", "Starting to search for updates", "locale", locale)

	is_default := 0

	var results []map[string]string

	session, err := windowsupdate.NewUpdateSession()
	if err != nil {
		return nil, errors.Wrap(err, "NewUpdateSession")
	}

	// If a specific locale is requested, set it.
	if locale == "_default" {
		is_default = 1
	} else {
		requestedLocale, err := strconv.ParseUint(locale, 10, 32)
		if err != nil {
			return nil, errors.Wrapf(err, "Parse locale %s", locale)
		}
		if err := session.SetLocal(uint32(requestedLocale)); err != nil {
			return nil, errors.Wrapf(err, "setting local to %d", uint32(requestedLocale))
		}
	}

	// What local is this data for? If it doesn't match the
	// requested one, throw an error, since sqlite is going to
	// block it.
	getLocale, err := session.GetLocal()
	if err != nil {
		return nil, errors.Wrap(err, "getlocale")
	}
	if strconv.FormatUint(uint64(getLocale), 10) != locale && is_default == 0 {
		return nil, errors.Wrapf(err, "set locale(%s) doesn't match returned locale(%d) sqlite will filter", locale, uint32(getLocale))
	} else {
		locale = strconv.FormatUint(uint64(getLocale), 10)
	}

	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return nil, errors.Wrap(err, "new searcher")
	}

	searchResults, err := searcher.Search("Type='Software'")
	if err != nil {
		return nil, errors.Wrap(err, "search")
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flatData, err := t.flattenOutput(dataQuery, searchResults)
		if err != nil {
			level.Info(t.logger).Log("msg", "flatten failed", "err", err)
			continue
		}

		rowData := map[string]string{
			"locale":     locale,
			"is_default": strconv.Itoa(is_default),
		}

		results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
	}

	level.Debug(t.logger).Log("msg", "Found updates", "locale", locale, "count", len(results))

	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, searchResults *windowsupdate.ISearchResult) ([]dataflatten.Row, error) {
	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts,
			dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())),
		)
	}

	// Works better if we bounce through json. yuck
	jsonBytes, err := json.Marshal(searchResults)
	if err != nil {
		return nil, errors.Wrap(err, "json")
	}

	return dataflatten.Json(jsonBytes, flattenOpts...)
}
