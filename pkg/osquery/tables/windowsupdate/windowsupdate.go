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

const enLocale = "1033" // English

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

	// Handling locale is a bit gnarly. If not specified, we want
	// to get the native locale, but we _also_ want to ensure we
	// get enLocale. But, when a locale is specified, we must only
	// fetch those, else sqlite will filter it out for us.
	needEnLocale := true
	for _, locale := range tablehelpers.GetConstraints(queryContext, "locale", tablehelpers.WithDefaults("default")) {
		gotEnLocale, result, err := t.searchLocale(locale, queryContext)
		if err != nil {
			level.Info(t.logger).Log("msg", "got error searching for updates", "locale", locale, "err", err)
			continue
		}
		results = append(results, result...)

		if gotEnLocale {
			needEnLocale = false
		}
	}

	if needEnLocale {
		if _, result, err := t.searchLocale(enLocale, queryContext); err != nil {
			level.Info(t.logger).Log("msg", "got error searching for updates on default", "locale", enLocale, "err", err)
		} else {
			results = append(results, result...)
		}
	}

	return results, nil
}

func (t *Table) searchLocale(locale string, queryContext table.QueryContext) (bool, []map[string]string, error) {
	level.Debug(t.logger).Log("msg", "Starting to search for updates", "locale", locale)

	fetchedEnLocal := false
	is_default := 0

	var results []map[string]string

	session, err := windowsupdate.NewUpdateSession()
	if err != nil {
		return fetchedEnLocal, nil, errors.Wrap(err, "NewUpdateSession")
	}

	if locale == "default" {
		is_default = 1
		var err error
		localeUint, err := session.GetLocal()
		if err != nil {
			return fetchedEnLocal, nil, errors.Wrap(err, "getlocale")
		}
		locale = strconv.FormatUint(uint64(localeUint), 10)
	} else {
		origLocale, err := session.GetLocal()
		if err != nil {
			return fetchedEnLocal, nil, errors.Wrapf(err, "Get locale")
		}

		localeUint, err := strconv.ParseUint(locale, 10, 32)
		if err != nil {
			return fetchedEnLocal, nil, errors.Wrapf(err, "Parse locale %s", locale)
		}
		if err := session.SetLocal(uint32(localeUint)); err != nil {
			return fetchedEnLocal, nil, errors.Wrapf(err, "setting local to %d", uint32(localeUint))
		}

		if origLocale == uint32(localeUint) {
			is_default = 1
		}

		// If the locale is specified, treat it like we've also gotten enLocale, since sqlite will filter it out.
		fetchedEnLocal = true
	}

	if locale == enLocale {
		fetchedEnLocal = true
	}

	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return fetchedEnLocal, nil, errors.Wrap(err, "new searcher")
	}

	searchResults, err := searcher.Search("Type='Software'")
	if err != nil {
		return fetchedEnLocal, nil, errors.Wrap(err, "search")
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

	return fetchedEnLocal, results, nil
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
