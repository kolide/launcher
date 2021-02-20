package osquery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
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

type writeFunction func(ctx context.Context, l logger.LogType, results []string, reeenroll bool) error

const initialRunnerResultsBatchSize = 10

func (i *initialRunner) Execute(configBlob string, writeFn writeFunction) error {
	// Sleep before starting to hammer on osquery
	time.Sleep(5 * time.Second)

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

	// TODO: Why do we have this? It feels like overhead to remove.
	toRun, err := i.queriesToRun(allQueries)
	if err != nil {
		return errors.Wrap(err, "checking if query should run")
	}

	var initialRunResults []OsqueryResultLog
	for packName, pack := range config.Packs {
		if !i.enabled { // only execute them when the plugin is enabled.
			break
		}

		// hack for sorting
		queryNames := []string{}
		queryContents := map[string]QueryContent{}

		for query, queryContent := range pack.Queries {
			queryName := fmt.Sprintf("pack:%s:%s", packName, query)
			if _, ok := toRun[queryName]; !ok {
				continue
			}

			// FIXME: remove this after testing.
			toSkip := false
			skipMe := []string{
				// "apps", // too slow
				"icloudsettingsset", // too slow
				"homebrewpackages",  // too slow
				"kernelextensions",  // FIXME: Why is this slow?
				"softwareupdate",    // But why? This isn't slow
				"tccentries",        // no such table, and we have an exit 1 in here.
			}
			for _, skipString := range skipMe {
				if strings.Contains(queryName, skipString) {
					toSkip = true
					break
				}
			}
			if toSkip {
				continue
			}

			queryNames = append(queryNames, queryName)
			queryContents[queryName] = queryContent
		}

		sort.Strings(queryNames)

		for _, queryName := range queryNames {
			queryContent := queryContents[queryName]
			queryStartTime := time.Now()
			// NOTE: This seems to have a 5s timeout
			resp, err := i.client.Query(queryContent.Query)
			// returning here causes the rest of the queries not to run
			// this is a bummer because often configs have queries with bad syntax/tables that do not exist.
			// log the error and move on.
			// using debug to not fill disks. the worst that will happen is that the result will come in later.
			level.Debug(i.logger).Log(
				"msg", "querying for initial results",
				"query_name", queryName,
				"sql", queryContent.Query,
				"err", err,
				"results", len(resp),
				"query time", time.Since(queryStartTime),
			)

			// FIXME: remove this block
			if err != nil {
				os.Exit(1)
			}

			if err != nil || len(resp) == 0 {
				continue
			}

			results := OsqueryResultLog{
				Name:           queryName,
				HostIdentifier: i.identifier,
				UnixTime:       int(time.Now().UTC().Unix()),
			}

			// Format this as either a snapshot or a diff
			// TODO: verify formatting on snapshots
			if queryContent.Snapshot == nil {
				results.DiffResults = &DiffResults{Added: resp}
			} else {
				results.Snapshot = resp
			}

			initialRunResults = append(initialRunResults, results)

			// Batch sending the responses back
			if len(initialRunResults) > initialRunnerResultsBatchSize {
				i.sendResults(writeFn, initialRunResults)
				initialRunResults = []OsqueryResultLog{}

				// Extra sleep for the batch to clear
				time.Sleep(10 * time.Second)

			}

			// Don't overwhelm the socket
			time.Sleep(1 * time.Second)
		}
	}

	// Any trailing jobs outside the batch?
	if len(initialRunResults) > 0 {
		i.sendResults(writeFn, initialRunResults)
	}

	// note: caching would happen always on first use, even if the runner is not enabled.
	// This avoids the problem of queries not being known even though they've been in the config for a long time.
	if err := i.cacheRanQueries(toRun); err != nil {
		return err
	}

	return nil
}

// sendResults takes a given batch of results, and sends them to the
// server. As this is meant to run in a loop, errors are mostly ignored.
func (i *initialRunner) sendResults(writeFn writeFunction, runResults []OsqueryResultLog) error {
	// TODO: Is this timeout an issue? It doesn't seem to be what we're
	// running into. Instead, we're running into something on the
	// client.Query path.
	cctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for _, result := range runResults {
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
