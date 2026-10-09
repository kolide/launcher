package filewalker

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/storage"
	storageci "github.com/kolide/launcher/v2/ee/agent/storage/ci"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

// Filewalk should roll up expected errors rather than logging them individually.
func TestFilewalk_ExpectedErrorsRolledUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	setupExpectedWalkErrors(t, root)

	var records []slog.Record
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	fw := newTestFilewalker(t, t.Name(), filewalkConfig{
		WalkInterval: duration(1 * time.Minute),
		filewalkDefinition: filewalkDefinition{
			RootDirs: &[]string{root},
		},
	}, store, slog.New(recordCapture{&records}))
	records = nil

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Go(func() { churnDirs(ctx, root) })
	const walks = 50
	for range walks {
		fw.Filewalk(t.Context())
	}
	cancel()
	wg.Wait()

	// Each walk should log exactly once at info+, with the rolled up errors.
	sawErrors := false
	for _, r := range records {
		require.Equal(t, slog.LevelInfo.String(), r.Level.String(), r.Message)
		var counts map[string]int
		r.Attrs(func(a slog.Attr) bool {
			if a.Key != "error_counts" {
				return true
			}
			counts, _ = a.Value.Any().(map[string]int)
			return false
		})
		require.NotNil(t, counts, r.Message)
		sawErrors = sawErrors || len(counts) > 0
	}
	require.Len(t, records, walks)
	require.True(t, sawErrors)
}

// Boilerplate to allow log inspection.
type recordCapture struct{ records *[]slog.Record }

func (recordCapture) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelInfo }
func (h recordCapture) WithAttrs([]slog.Attr) slog.Handler         { return h }
func (h recordCapture) WithGroup(string) slog.Handler              { return h }
func (h recordCapture) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r.Clone())
	return nil
}

// Races the walker by constantly deleting dirs and swapping them for files, sometimes
// interrupting it mid-walk.
func churnDirs(ctx context.Context, root string) {
	for i := 0; ctx.Err() == nil; i++ {
		p := filepath.Join(root, "churn"+strconv.Itoa(i%16))
		_ = os.MkdirAll(filepath.Join(p, "sub"), 0o755)
		_ = os.RemoveAll(p)
		if i%2 == 0 {
			_ = os.WriteFile(p, nil, 0o644)
			_ = os.Remove(p)
		}
	}
}
