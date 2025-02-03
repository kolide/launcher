package tablewrapper

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
	"golang.org/x/sync/semaphore"
)

const (
	DefaultTableTimeout = 4 * time.Minute
	numWorkers          = 5
)

type wrappedTable struct {
	flagsController types.Flags
	slogger         *slog.Logger
	name            string
	gen             table.GenerateFunc
	genTimeout      time.Duration
	genTimeoutLock  *sync.Mutex
	workers         *semaphore.Weighted
}

type tablePluginOption func(*wrappedTable)

// WithGenerateTimeout overrides the default table timeout of four minutes.
// The control server may override this value.
func WithGenerateTimeout(genTimeout time.Duration) tablePluginOption {
	return func(w *wrappedTable) {
		w.genTimeout = genTimeout
	}
}

type generateResult struct {
	rows []map[string]string
	err  error
}

// New returns a table plugin that will attempt to execute a query up until the given timeout,
// at which point it will instead return no rows and a timeout error.
func New(flags types.Flags, slogger *slog.Logger, name string, columns []table.ColumnDefinition, gen table.GenerateFunc, opts ...tablePluginOption) *table.Plugin {
	wt := newWrappedTable(flags, slogger, name, gen, opts...)
	return table.NewPlugin(name, columns, wt.generate) //nolint:forbidigo // This is our one allowed usage of table.NewPlugin
}

// newWrappedTable returns a new `wrappedTable`. We split the constructor out for ease of testing
// specific wrappedTable functionality around flag changes.
func newWrappedTable(flags types.Flags, slogger *slog.Logger, name string, gen table.GenerateFunc, opts ...tablePluginOption) *wrappedTable {
	wt := &wrappedTable{
		flagsController: flags,
		slogger:         slogger.With("table_name", name),
		name:            name,
		gen:             gen,
		genTimeout:      flags.GenerateTimeout(),
		genTimeoutLock:  &sync.Mutex{},
		workers:         semaphore.NewWeighted(numWorkers),
	}

	for _, opt := range opts {
		opt(wt)
	}

	flags.RegisterChangeObserver(wt, keys.GenerateTimeout)

	return wt
}

// FlagsChanged satisfies the types.FlagsChangeObserver interface -- handles updates to flags
// that we care about, which is just `GenerateTimeout`
func (wt *wrappedTable) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	if !slices.Contains(flagKeys, keys.GenerateTimeout) {
		return
	}

	wt.genTimeoutLock.Lock()
	defer wt.genTimeoutLock.Unlock()

	newGenTimeout := wt.flagsController.GenerateTimeout()

	wt.slogger.Log(ctx, slog.LevelInfo,
		"received changed value for generate_timeout",
		"old_timeout", wt.genTimeout.String(),
		"new_timeout", newGenTimeout.String(),
	)

	wt.genTimeout = newGenTimeout
}

// generate wraps `wt.gen`, ensuring the function is traced and that it does not run for longer
// than `wt.genTimeout`.
func (wt *wrappedTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", wt.name, "generate_timeout", wt.genTimeout.String())
	defer span.End()

	// Get the current timeout value -- this value can change per the control server
	wt.genTimeoutLock.Lock()
	genTimeout := wt.genTimeout
	wt.genTimeoutLock.Unlock()

	ctx, cancel := context.WithTimeout(ctx, genTimeout)
	defer cancel()

	// A worker must be available for us to try to run the generate function --
	// we don't want too many calls to the same table piling up.
	if !wt.workers.TryAcquire(1) {
		span.AddEvent("no_workers_available")
		return nil, fmt.Errorf("no workers available (limit %d)", numWorkers)
	}

	// Kick off running the query
	resultChan := make(chan *generateResult)
	gowrapper.Go(ctx, wt.slogger, func() {
		rows, err := wt.gen(ctx, queryContext)
		wt.workers.Release(1)
		span.AddEvent("generate_returned")
		resultChan <- &generateResult{
			rows: rows,
			err:  err,
		}
	})

	// Wait for results up until the timeout
	select {
	case result := <-resultChan:
		return result.rows, result.err
	case <-ctx.Done():
		queriedColumns := columnsFromConstraints(queryContext)
		wt.slogger.Log(ctx, slog.LevelWarn,
			"query timed out",
			"queried_columns", fmt.Sprintf("%+v", queriedColumns),
		)
		return nil, fmt.Errorf("querying %s timed out after %s (queried columns: %v)", wt.name, genTimeout.String(), queriedColumns)
	}
}

// columnsFromConstraints extracts the column names from the query for logging purposes
func columnsFromConstraints(queryContext table.QueryContext) []string {
	keys := make([]string, 0, len(queryContext.Constraints))
	for k := range queryContext.Constraints {
		keys = append(keys, k)
	}

	return keys
}
