package osquery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/osquery/osquery-go/plugin/logger"
)

type initialRunner struct {
	slogger    *slog.Logger
	enabled    bool
	identifier string
	client     Querier
	store      types.GetterSetter
}

func (i *initialRunner) Execute(configBlob string, writeFn func(ctx context.Context, l logger.LogType, results []string, reeenroll bool) error) error {
	if !i.enabled {
		i.slogger.Log(context.TODO(), slog.LevelDebug,
			"initial runner not enabled",
		)
		return nil
	}
	var config OsqueryConfig
	if err := json.Unmarshal([]byte(configBlob), &config); err != nil {
		return fmt.Errorf("unmarshal osquery config blob: %w", err)
	}

	var allQueries []string
	for packName, pack := range config.Packs {
		// only run queries from kolide packs
		if !strings.Contains(packName, "_kolide_") {
			continue
		}

		// Run all the queries, snapshot and differential
		for query := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			allQueries = append(allQueries, queryName)
		}
	}

	toRun, err := i.queriesToRun(allQueries)
	if err != nil {
		return fmt.Errorf("checking if query should run: %w", err)
	}

	var initialRunResults []OsqueryResultLog
	for packName, pack := range config.Packs {
		for query, queryContent := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			if _, ok := toRun[queryName]; !ok {
				continue
			}
			resp, err := i.client.Query(queryContent.Query)
			// returning here causes the rest of the queries not to run
			// this is a bummer because often configs have queries with bad syntax/tables that do not exist.
			// log the error and move on.
			// using debug to not fill disks. the worst that will happen is that the result will come in later.
			i.slogger.Log(context.TODO(), slog.LevelDebug,
				"querying for initial results",
				"query_name", queryName,
				"err", err,
				"results", len(resp),
			)
			if err != nil || len(resp) == 0 {
				continue
			}

			initialRunResults = append(initialRunResults, OsqueryResultLog{
				Name:           queryName,
				HostIdentifier: i.identifier,
				UnixTime:       int(time.Now().UTC().Unix()),
				DiffResults:    &DiffResults{Added: resp},
			})
		}
	}

	cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, result := range initialRunResults {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(result); err != nil {
			return fmt.Errorf("encoding initial run result: %w", err)
		}
		if err := writeFn(cctx, logger.LogTypeString, []string{buf.String()}, true); err != nil {
			i.slogger.Log(cctx, slog.LevelDebug,
				"writing initial result log to server",
				"query_name", result.Name,
				"err", err,
			)
			continue
		}
	}

	// note: caching would happen always on first use, even if the runner is not enabled.
	// This avoids the problem of queries not being known even though they've been in the config for a long time.
	if err := i.cacheRanQueries(toRun); err != nil {
		return err
	}

	return nil
}

func (i *initialRunner) queriesToRun(allFromConfig []string) (map[string]struct{}, error) {
	known := make(map[string]struct{})

	for _, q := range allFromConfig {
		knownQuery, err := i.store.Get([]byte(q))
		if err != nil {
			return nil, fmt.Errorf("check store for queries to run: %w", err)
		}
		if knownQuery != nil {
			continue
		}
		known[q] = struct{}{}
	}

	return known, nil
}

func (i *initialRunner) cacheRanQueries(known map[string]struct{}) error {
	for q := range known {
		if err := i.store.Set([]byte(q), []byte(q)); err != nil {
			return fmt.Errorf("cache initial result query %q: %w", q, err)
		}
	}

	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
