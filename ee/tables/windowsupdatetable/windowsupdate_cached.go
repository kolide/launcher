//go:build windows
// +build windows

package windowsupdatetable

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

type CachedWindowsUpdatesTable struct {
	flags      types.Flags
	slogger    *slog.Logger
	cacheStore types.Getter
	name       string
}

func CachedWindowsUpdatesTablePlugin(flags types.Flags, slogger *slog.Logger, cacheStore types.Getter) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("locale"),
		table.IntegerColumn("is_default"),
		table.IntegerColumn("is_cached"),
		table.IntegerColumn("age"),
	)

	t := &CachedWindowsUpdatesTable{
		flags:      flags,
		slogger:    slogger.With("name", "kolide_windows_updates_cached"),
		cacheStore: cacheStore,
		name:       "kolide_windows_updates_cached",
	}

	return tablewrapper.New(flags, slogger, t.name, columns, t.generateFromCachedData)
}

func (c *CachedWindowsUpdatesTable) generateFromCachedData(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", c.name)
	defer span.End()

	var results []map[string]string

	for _, locale := range tablehelpers.GetConstraints(queryContext, "locale", tablehelpers.WithDefaults(defaultLocale)) {
		rawLocaleResults, err := c.cacheStore.Get([]byte(locale))
		if err != nil {
			c.slogger.Log(ctx, slog.LevelWarn,
				"could not get cached data for locale",
				"locale", locale,
				"err", err,
			)
			continue
		}
		if len(rawLocaleResults) == 0 {
			c.slogger.Log(ctx, slog.LevelWarn,
				"no cached data set for locale",
				"locale", locale,
			)
			continue
		}

		var res cachedQueryResults
		if err := json.Unmarshal(rawLocaleResults, &res); err != nil {
			c.slogger.Log(ctx, slog.LevelWarn,
				"could not unmarshal cached data for locale",
				"locale", locale,
				"err", err,
			)
			continue
		}

		if res.QueryTime.Add(c.flags.CachedQueryResultsTTL()).Before(time.Now()) {
			c.slogger.Log(ctx, slog.LevelWarn,
				"cached data is expired, not using",
				"locale", locale,
				"cached_data_query_time", res.QueryTime.String(),
			)
			continue
		}

		resultsAgeInSecondsStr := strconv.Itoa(int(time.Now().Unix() - res.QueryTime.Unix()))

		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			flattenOpts := []dataflatten.FlattenOpts{
				dataflatten.WithSlogger(c.slogger),
				dataflatten.WithQuery(strings.Split(dataQuery, "/")),
			}

			flatData, err := dataflatten.Json(res.Results.RawResults, flattenOpts...)
			if err != nil {
				c.slogger.Log(ctx, slog.LevelInfo,
					"flatten failed",
					"err", err,
				)
				continue
			}

			rowData := map[string]string{
				"locale":     res.Results.Locale,
				"is_default": strconv.Itoa(res.Results.IsDefaultLocale),
				"is_cached":  "1",
				"age":        resultsAgeInSecondsStr,
			}

			results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
		}
	}

	return results, nil
}
