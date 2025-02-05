//go:build windows
// +build windows

package windowsupdatetable

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/kolide/launcher/pkg/windows/windowsupdate"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/scjalliance/comshim"
)

type tableMode int

const (
	UpdatesTable tableMode = iota
	HistoryTable
)

type Table struct {
	slogger   *slog.Logger
	queryFunc queryFuncType
	name      string
}

func TablePlugin(mode tableMode, flags types.Flags, slogger *slog.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		table.TextColumn("locale"),
		table.IntegerColumn("is_default"),
	)

	t := &Table{}

	switch mode {
	case UpdatesTable:
		t.queryFunc = queryUpdates
		t.name = "kolide_windows_updates"
	case HistoryTable:
		t.queryFunc = queryHistory
		t.name = "kolide_windows_update_history"
	}

	t.slogger = slogger.With("name", t.name)

	return tablewrapper.New(flags, slogger, t.name, columns, t.generate)
}

func queryUpdates(searcher *windowsupdate.IUpdateSearcher) (interface{}, error) {
	return searcher.Search("Type='Software'")
}

func queryHistory(searcher *windowsupdate.IUpdateSearcher) (interface{}, error) {
	return searcher.QueryHistoryAll()
}

type queryFuncType func(*windowsupdate.IUpdateSearcher) (interface{}, error)

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", t.name)
	defer span.End()

	var results []map[string]string

	for _, locale := range tablehelpers.GetConstraints(queryContext, "locale", tablehelpers.WithDefaults("_default")) {
		result, err := t.searchLocale(ctx, locale, queryContext)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"got error searching",
				"locale", locale,
				"err", err,
			)
			continue
		}
		results = append(results, result...)

	}

	return results, nil

}

func (t *Table) searchLocale(ctx context.Context, locale string, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	comshim.Add(1)
	defer comshim.Done()

	var results []map[string]string

	searcher, setLocale, isDefaultLocale, err := getSearcher(locale)
	if err != nil {
		return nil, fmt.Errorf("new searcher: %w", err)
	}

	searchResults, err := t.queryFunc(searcher)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flatData, err := t.flattenOutput(dataQuery, searchResults)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"flatten failed",
				"err", err,
			)
			continue
		}

		rowData := map[string]string{
			"locale":     setLocale,
			"is_default": strconv.Itoa(isDefaultLocale),
		}

		results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
	}

	return results, nil
}

func (t *Table) flattenOutput(dataQuery string, searchResults interface{}) ([]dataflatten.Row, error) {
	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	// dataflatten won't parse the raw searchResults. As a workaround,
	// we marshal to json. This is a deficiency in dataflatten.
	jsonBytes, err := json.Marshal(searchResults)
	if err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	return dataflatten.Json(jsonBytes, flattenOpts...)
}

func getSearcher(locale string) (*windowsupdate.IUpdateSearcher, string, int, error) {
	isDefaultLocale := 0

	session, err := windowsupdate.NewUpdateSession()
	if err != nil {
		return nil, locale, isDefaultLocale, fmt.Errorf("NewUpdateSession: %w", err)
	}

	// If a specific locale is requested, set it.
	if locale == "_default" {
		isDefaultLocale = 1
	} else {
		requestedLocale, err := strconv.ParseUint(locale, 10, 32)
		if err != nil {
			return nil, locale, isDefaultLocale, fmt.Errorf("Parse locale %s: %w", locale, err)
		}
		if err := session.SetLocal(uint32(requestedLocale)); err != nil {
			return nil, locale, isDefaultLocale, fmt.Errorf("setting local to %d: %w", uint32(requestedLocale), err)
		}
	}

	// What local is this data for? If it doesn't match the
	// requested one, throw an error, since sqlite is going to
	// block it.
	getLocale, err := session.GetLocal()
	if err != nil {
		return nil, locale, isDefaultLocale, fmt.Errorf("getlocale: %w", err)
	}
	if strconv.FormatUint(uint64(getLocale), 10) != locale && isDefaultLocale == 0 {
		return nil, locale, isDefaultLocale, fmt.Errorf("set locale(%s) doesn't match returned locale(%d) sqlite will filter: %w", locale, getLocale, err)
	} else {
		locale = strconv.FormatUint(uint64(getLocale), 10)
	}

	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return nil, locale, isDefaultLocale, fmt.Errorf("new searcher: %w", err)
	}

	return searcher, locale, isDefaultLocale, err
}
