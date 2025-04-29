//go:build windows
// +build windows

package windowsupdatetable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/windows/windowsupdate"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/scjalliance/comshim"
)

// QueryResults is the data returned by execing `launcher.exe query-windowsupdates`.
type QueryResults struct {
	RawResults      []byte `json:"raw_results"`
	Locale          string `json:"locale"`
	IsDefaultLocale int    `json:"is_default_locale"`
	ErrStr          string `json:"err_string"`
}

type tableMode int

const (
	UpdatesTable tableMode = iota
	HistoryTable

	defaultLocale = "_default"
)

type Table struct {
	slogger   *slog.Logger
	queryFunc queryFuncType
	name      string
	mode      tableMode
}

func TablePlugin(mode tableMode, flags types.Flags, slogger *slog.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		table.TextColumn("locale"),
		table.IntegerColumn("is_default"),
	)

	t := &Table{
		mode: mode,
	}

	switch mode {
	case UpdatesTable:
		t.queryFunc = queryUpdates
		t.name = "kolide_windows_updates"
	case HistoryTable:
		t.queryFunc = queryHistory
		t.name = "kolide_windows_update_history"
	}

	t.slogger = slogger.With("name", t.name)

	return tablewrapper.New(flags, slogger, t.name, columns, t.generateWithLauncherExec)
}

func queryUpdates(searcher *windowsupdate.IUpdateSearcher) (interface{}, error) {
	return searcher.Search("Type='Software'")
}

func queryHistory(searcher *windowsupdate.IUpdateSearcher) (interface{}, error) {
	return searcher.QueryHistoryAll()
}

type queryFuncType func(*windowsupdate.IUpdateSearcher) (interface{}, error)

// generateWithLauncherExec replaces the previous `generate` function. It shells out
// to launcher's new `query-windowsupdates` subcommand, which performs the actual query
// and writes JSON to stdout for this generate function to consume. We do this because
// we suspect a memory leak when calling `IUpdateSearcher.Search` -- if we call this function
// in a new launcher process, then the memory will be released when that process terminates.
// It's not an ideal long-term solution, but we are hoping it helps with memory issues in the
// short term as we track down the issue in go-ole. Note that we only suspect memory leak
// issues in the `Search` function and not in `QueryHistoryAll` -- but to be safe and for ease
// of implementation, we are moving both to launcher execs.
func (t *Table) generateWithLauncherExec(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", t.name)
	defer span.End()

	launcherPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("getting path to launcher: %w", err)
	}
	if !strings.HasSuffix(launcherPath, "launcher.exe") {
		return nil, errors.New("cannot run generate for non-launcher executable (is this running in a test context?)")
	}

	var results []map[string]string

	for _, locale := range tablehelpers.GetConstraints(queryContext, "locale", tablehelpers.WithDefaults(defaultLocale)) {
		res, err := callQueryWindowsUpdatesSubcommand(ctx, launcherPath, locale, t.mode)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelWarn,
				"error running query windows updates subcommand",
				"err", err,
			)
			continue
		}

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(t.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flatData, err := dataflatten.Json(res.RawResults, flattenOpts...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"flatten failed",
					"err", err,
				)
				continue
			}

			rowData := map[string]string{
				"locale":     res.Locale,
				"is_default": strconv.Itoa(res.IsDefaultLocale),
			}

			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
		}
	}

	return results, nil
}

// callQueryWindowsUpdatesSubcommand performs the required query by execing `launcher.exe query-windowsupdates`;
// it unmarshals the results into the expected format. We perform the exec to handle a memory leak in the Search
// function. If a timeout has been set on `ctx`, it will be respected by the underlying exec.
func callQueryWindowsUpdatesSubcommand(ctx context.Context, launcherPath string, locale string, mode tableMode) (*QueryResults, error) {
	ctx, span := observability.StartSpan(ctx, "locale", locale, "table_mode", int(mode))
	defer span.End()

	args := []string{
		"query-windowsupdates",
		"-locale", locale,
		"-table_mode", strconv.Itoa(int(mode)),
	}

	cmd := exec.CommandContext(ctx, launcherPath, args...) //nolint:forbidigo // We can exec the current executable safely
	cmd.Env = append(cmd.Env, "LAUNCHER_SKIP_UPDATES=TRUE")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running query-windowsupdates: %w", err)
	}

	var res QueryResults
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("unmarshalling results of running launcher query-windowsupdates: %w", err)
	}

	if res.ErrStr != "" {
		return nil, fmt.Errorf("launcher query-windowsupdates returned error: %s", res.ErrStr)
	}

	return &res, nil
}

//nolint:unused
func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", t.name)
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

//nolint:unused
func (t *Table) searchLocale(ctx context.Context, locale string, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx)
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

//nolint:unused
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

//nolint:unused
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
