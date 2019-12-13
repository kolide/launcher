package osquery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

type initialRunner struct {
	logger     log.Logger
	enabled    bool
	identifier string
	client     Querier
	db         *bolt.DB
}

func (i *initialRunner) Execute(configBlob string, writeFn func(ctx context.Context, l logger.LogType, results []string, reeenroll bool) error) error {
	var config OsqueryConfig
	if err := json.Unmarshal([]byte(configBlob), &config); err != nil {
		return errors.Wrap(err, "unmarshal osquery config blob")
	}

	var allQueries []string
	for packName, pack := range config.Packs {
		// only run queries from kolide packs
		if !strings.Contains(packName, "_kolide_") {
			continue
		}

		// Run all the queries, snapshot and differential
		for query, _ := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			allQueries = append(allQueries, queryName)
		}
	}

	toRun, err := i.queriesToRun(allQueries)
	if err != nil {
		return errors.Wrap(err, "checking if query should run")
	}

	var initialRunResults []OsqueryResultLog
	for packName, pack := range config.Packs {
		if !i.enabled { // only execute them when the plugin is enabled.
			break
		}
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
			level.Debug(i.logger).Log(
				"msg", "querying for initial results",
				"query_name", queryName,
				"err", err,
				"results", len(resp),
			)
			if err != nil || len(resp) == 0 {
				continue
			}

			results := OsqueryResultLog{
				Name:           queryName,
				HostIdentifier: i.identifier,
				UnixTime:       int(time.Now().UTC().Unix()),
			}

			// Format this as either a snapshot or a diff
			if queryContent.Snapshot == nil {
				results.DiffResults = &DiffResults{Added: resp}
			} else {
				results.Snapshot = resp
			}

			initialRunResults = append(initialRunResults, results)
		}
	}

	cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, result := range initialRunResults {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(result); err != nil {
			return errors.Wrap(err, "encoding initial run result")
		}
		if err := writeFn(cctx, logger.LogTypeString, []string{buf.String()}, true); err != nil {
			level.Debug(i.logger).Log(
				"msg", "writing initial result log to server",
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
	err := i.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(initialResultsBucket))
		for _, q := range allFromConfig {
			knownQuery := b.Get([]byte(q))
			if knownQuery != nil {
				continue
			}
			known[q] = struct{}{}
		}
		return nil
	})

	return known, errors.Wrap(err, "check bolt for queries to run")
}

func (i *initialRunner) cacheRanQueries(known map[string]struct{}) error {
	err := i.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(initialResultsBucket))
		for q := range known {
			if err := b.Put([]byte(q), []byte(q)); err != nil {
				return errors.Wrapf(err, "cache initial result query %q", q)
			}
		}
		return nil
	})
	return errors.Wrap(err, "caching known initial result queries")
}
