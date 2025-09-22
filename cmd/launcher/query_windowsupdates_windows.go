//go:build windows
// +build windows

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	comshim "github.com/NozomiNetworks/go-comshim"
	"github.com/kolide/launcher/ee/tables/windowsupdatetable"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/windows/windowsupdate"
	"github.com/peterbourgon/ff/v3"
)

// runQueryWindowsUpdates is a subcommand allowing us to call the Windows Update Agent
// API without experiencing memory leak issues -- see documentation in the
// windowsupdatetable package for more details. This is a short-term solution.
func runQueryWindowsUpdates(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	var (
		flagset     = flag.NewFlagSet("query-windowsupdates", flag.ExitOnError)
		flLocale    = flagset.String("locale", "_default", "search locale")
		flTableMode = flagset.Int("table_mode", int(windowsupdatetable.UpdatesTable), fmt.Sprintf("updates table (%d); history table (%d)", windowsupdatetable.UpdatesTable, windowsupdatetable.HistoryTable))
	)

	if err := ff.Parse(flagset, args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	rawResults, locale, isDefaultLocale, searchErr := searchLocale(*flLocale, *flTableMode)
	queryResults := &windowsupdatetable.QueryResults{
		RawResults:      rawResults,
		Locale:          locale,
		IsDefaultLocale: isDefaultLocale,
	}
	if searchErr != nil {
		queryResults.ErrStr = searchErr.Error()
	}

	queryResultsBytes, err := json.Marshal(queryResults)
	if err != nil {
		return fmt.Errorf("marshalling response: %w", err)
	}

	if _, err := os.Stdout.Write(queryResultsBytes); err != nil {
		return fmt.Errorf("writing results: %w", err)
	}

	return nil
}

func searchLocale(locale string, tableMode int) ([]byte, string, int, error) {
	if err := comshim.TryAdd(1); err != nil {
		comshim.Done() // ensure we decrement the global shim counter that TryAdd increments immediately
		return nil, "", 0, fmt.Errorf("unable to init comshim: %w", err)
	}
	defer comshim.Done()

	searcher, setLocale, isDefaultLocale, err := getSearcher(locale)
	if err != nil {
		return nil, "", 0, fmt.Errorf("new searcher: %w", err)
	}

	var searchResults interface{}
	if tableMode == int(windowsupdatetable.UpdatesTable) {
		searchResults, err = searcher.Search("Type='Software'")
	} else if tableMode == int(windowsupdatetable.HistoryTable) {
		searchResults, err = searcher.QueryHistoryAll()
	} else {
		return nil, "", 0, fmt.Errorf("unsupported table mode %d", tableMode)
	}

	if err != nil {
		return nil, "", 0, fmt.Errorf("querying: %w", err)
	}

	// dataflatten won't parse the raw searchResults. As a workaround,
	// we marshal to json. This is a deficiency in dataflatten.
	jsonBytes, err := json.Marshal(searchResults)
	if err != nil {
		return nil, "", 0, fmt.Errorf("json: %w", err)
	}

	return jsonBytes, setLocale, isDefaultLocale, nil
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

	// What locale is this data for? If it doesn't match the
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

	return searcher, locale, isDefaultLocale, nil
}
