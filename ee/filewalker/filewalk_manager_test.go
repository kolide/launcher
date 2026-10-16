package filewalker

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/storage"
	storageci "github.com/kolide/launcher/v2/ee/agent/storage/ci"
	"github.com/kolide/launcher/v2/ee/agent/types"
	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

const (
	dotenvCfg   = `{"walk_interval": "2h", "root_dirs": ["$ROOT/home/*"], "file_name_regex": "^\\.env$", "file_type_filter": "file"}`
	hostKeysCfg = `{"walk_interval": "4h", "root_dirs": ["$ROOT/etc/ssh"], "file_name_regex": "^ssh_host_.+_key$", "file_type_filter": "file"}`
)

func TestManager_WalksNeverWalkedAtStartup(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)

		h.start()

		require.Equal(t, time.Now().Unix(), h.lastWalk("dotenv"))
		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
		require.Equal(t, []string{"home/kiwi/.env"}, h.results("dotenv"))
		require.Equal(t, []string{"etc/ssh/ssh_host_ed25519_key"}, h.results("host_keys"))
	})
}

// A config walked recently before startup should wait until it is due, then walk within one tick.
func TestManager_WaitsUntilDueAfterStartup(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("host_keys", hostKeysCfg)
		seeded := h.seedLastWalk("host_keys", 4*time.Hour-30*time.Minute)

		h.start()
		h.advance(25 * time.Minute)
		require.Equal(t, seeded, h.lastWalk("host_keys"))

		h.advance(walkCheckInterval)
		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
	})
}

func TestManager_WalksOnSchedule(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)

		h.start()
		h.advance(8 * time.Hour)

		require.Equal(t, 5, h.walks("dotenv"))
		require.Equal(t, 3, h.walks("host_keys"))
	})
}

// Ping should walk every config immediately, even those not due.
func TestManager_PingWalksAllImmediately(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()
		bootWalk := time.Now().Unix()

		h.advance(walkCheckInterval)
		require.Equal(t, bootWalk, h.lastWalk("dotenv"))
		require.Equal(t, bootWalk, h.lastWalk("host_keys"))

		h.fm.Ping()
		synctest.Wait()
		require.Equal(t, time.Now().Unix(), h.lastWalk("dotenv"))
		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
	})
}

// Several Pings or Dos before a tick should produce a single full walk.
func TestManager_RequestsCoalesce(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.seedLastWalk("dotenv", 0)
		h.seedLastWalk("host_keys", 0)

		h.fm.Ping()
		require.NoError(t, h.do(`{"filewalks": []}`))
		h.fm.Ping()
		require.NoError(t, h.do(`{"filewalks": ["host_keys"]}`))

		h.start()
		require.Equal(t, 1, h.walks("dotenv"))
		require.Equal(t, 1, h.walks("host_keys"))

		h.advance(walkCheckInterval)
		require.Equal(t, 1, h.walks("dotenv"))
		require.Equal(t, 1, h.walks("host_keys"))
	})
}

// A ping should cancel any walk in flight. We check this
// by blocking on the attempt to set a last walk time.
func TestManager_PingCancelsInFlightWalk(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()

		release := h.resultsStore.holdSet(LastWalkTimeKey("dotenv"))
		require.NoError(t, h.do(`{"filewalks": []}`))
		synctest.Wait()

		h.fm.Ping()
		close(release)
		synctest.Wait()

		require.Equal(t, 3, h.walks("dotenv"))
		require.Equal(t, 2, h.walks("host_keys"))
	})
}

// Do defers to whatever is running. We check this
// by blocking on the attempt to set a last walk time.
func TestManager_DoDoesNotCancelInFlightWalk(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()

		release := h.resultsStore.holdSet(LastWalkTimeKey("dotenv"))
		require.NoError(t, h.do(`{"filewalks": []}`))
		synctest.Wait()

		require.NoError(t, h.do(`{"filewalks": []}`))
		close(release)
		synctest.Wait()

		require.Equal(t, 3, h.walks("dotenv"))
		require.Equal(t, 3, h.walks("host_keys"))
	})
}

func TestManager_DoEmptyWalksAll(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()

		h.advance(time.Minute)
		require.NoError(t, h.do(`{"filewalks": []}`))
		synctest.Wait()

		require.Equal(t, 2, h.walks("dotenv"))
		require.Equal(t, 2, h.walks("host_keys"))
	})
}

// FilewalkManager should pick up and use newly added configs on the next run.
func TestManager_WalksAddedConfig(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.start()

		h.setConfig("host_keys", hostKeysCfg)
		h.advance(walkCheckInterval)

		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
		require.Equal(t, []string{"etc/ssh/ssh_host_ed25519_key"}, h.results("host_keys"))
	})
}

// A removed config's results and last walk time should be deleted, and stay deleted.
func TestManager_PurgesRemovedConfig(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()

		require.NoError(t, h.cfgStore.Delete([]byte("host_keys")))
		h.advance(walkCheckInterval)
		require.False(t, h.stored("host_keys"))
		require.False(t, h.stored(string(LastWalkTimeKey("host_keys"))))

		h.advance(8 * time.Hour)
		require.False(t, h.stored("host_keys"))
		require.False(t, h.stored(string(LastWalkTimeKey("host_keys"))))
		require.True(t, h.stored("dotenv"))
	})
}

// A config that no longer parses should keep its previous results while other configs keep walking.
func TestManager_KeepsBrokenConfigResults(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()
		bootWalk := time.Now().Unix()

		h.advance(time.Minute)
		h.setConfig("host_keys", `{not json`)
		h.fm.Ping()
		synctest.Wait()

		require.Equal(t, time.Now().Unix(), h.lastWalk("dotenv"))
		require.Equal(t, bootWalk, h.lastWalk("host_keys"))
		require.Equal(t, []string{"etc/ssh/ssh_host_ed25519_key"}, h.results("host_keys"))
	})
}

// An invalid last walk should still get walked and then overwritten.
func TestManager_WalksInvalidLastWalk(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("host_keys", hostKeysCfg)
		require.NoError(t, h.resultsStore.KVStore.Set(LastWalkTimeKey("host_keys"), []byte("foobar")))

		h.start()

		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
	})
}

func TestManager_InterruptMultipleDoesNotBlock(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.start()

		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() { h.fm.Interrupt(nil) })
		}
		wg.Wait()
		synctest.Wait()

		require.True(t, h.stopped())
	})
}

// An early interrupt should be handled fine and stop anyway.
func TestManager_InterruptBeforeExecute(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.fm.Interrupt(nil)

		h.start()

		require.True(t, h.stopped())
	})
}

// due should return config names ready to walk.
func Test_due(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_000_000, 0)
	cfgs := map[string]filewalkConfig{
		"never_walked": {WalkInterval: duration(time.Hour)},
		"future":       {WalkInterval: duration(time.Hour)},
		"not_due":      {WalkInterval: duration(time.Hour)},
		"elapsed":      {WalkInterval: duration(time.Hour)},
	}
	lastWalks := map[string]time.Time{
		"future":  now.Add(24 * time.Hour), // clock moved back
		"not_due": now.Add(-59 * time.Minute),
		"elapsed": now.Add(-time.Hour),
	}

	require.ElementsMatch(t, []string{"elapsed", "future", "never_walked"}, due(now, cfgs, lastWalks))
}

type managerHarness struct {
	t            *testing.T
	root         string
	cfgStore     types.KVStore
	resultsStore *countingStore
	fm           *FilewalkManager
	done         chan struct{}
}

type countingStore struct {
	types.KVStore
	mu   sync.Mutex
	sets map[string]int
	hold map[string]chan struct{}
}

func (s *countingStore) Set(key, value []byte) error {
	s.mu.Lock()
	s.sets[string(key)]++
	release, held := s.hold[string(key)]
	delete(s.hold, string(key))
	s.mu.Unlock()
	if held {
		<-release
	}
	return s.KVStore.Set(key, value)
}

// next set on key blocks until chan closes
func (s *countingStore) holdSet(key []byte) chan struct{} {
	release := make(chan struct{})
	s.mu.Lock()
	s.hold[string(key)] = release
	s.mu.Unlock()
	return release
}

// runManagerTest runs body in a synctest bubble with a fresh manager; time only advances while every goroutine is blocked.
func runManagerTest(t *testing.T, body func(h *managerHarness)) {
	synctest.Test(t, func(t *testing.T) {
		slogger := multislogger.NewNopLogger()
		root := t.TempDir()
		require.NoError(t, os.CopyFS(root, fstest.MapFS{
			"etc/ssh/ssh_host_ed25519_key":     {},
			"etc/ssh/ssh_host_ed25519_key.pub": {},
			"home/kiwi/.env":                   {},
			"home/kiwi/notes.txt":              {},
		}))
		cfgStore, err := storageci.NewStore(t, slogger, storage.FilewalkConfigStore.String())
		require.NoError(t, err)
		store, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
		require.NoError(t, err)
		resultsStore := &countingStore{KVStore: store, sets: map[string]int{}, hold: map[string]chan struct{}{}}
		k := typesmocks.NewKnapsack(t)
		k.On("FilewalkConfigStore").Return(cfgStore)
		k.On("FilewalkResultsStore").Return(resultsStore)

		h := &managerHarness{
			t:            t,
			root:         root,
			cfgStore:     cfgStore,
			resultsStore: resultsStore,
			fm:           New(k, slogger),
		}
		defer h.stop()
		body(h)
	})
}

func (h *managerHarness) setConfig(name, cfg string) {
	escapedRoot, err := json.Marshal(h.root)
	require.NoError(h.t, err)
	cfg = strings.ReplaceAll(cfg, "$ROOT", strings.Trim(string(escapedRoot), `"`))
	require.NoError(h.t, h.cfgStore.Set([]byte(name), []byte(cfg)))
}

func (h *managerHarness) seedLastWalk(name string, ago time.Duration) int64 {
	ts := time.Now().Add(-ago).Unix()
	require.NoError(h.t, h.resultsStore.KVStore.Set(LastWalkTimeKey(name), binary.NativeEndian.AppendUint64(nil, uint64(ts))))
	return ts
}

func (h *managerHarness) start() {
	h.done = make(chan struct{})
	go func() {
		h.fm.Execute()
		close(h.done)
	}()
	synctest.Wait()
}

func (h *managerHarness) stop() {
	if h.done == nil {
		return
	}
	h.fm.Interrupt(nil)
	<-h.done
}

func (h *managerHarness) stopped() bool {
	select {
	case <-h.done:
		return true
	default:
		return false
	}
}

func (h *managerHarness) advance(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func (h *managerHarness) do(req string) error {
	return h.fm.Do(strings.NewReader(req))
}

func (h *managerHarness) stored(key string) bool {
	raw, err := h.resultsStore.Get([]byte(key))
	require.NoError(h.t, err)
	return raw != nil
}

func (h *managerHarness) lastWalk(name string) int64 {
	raw, err := h.resultsStore.Get(LastWalkTimeKey(name))
	require.NoError(h.t, err)
	if raw == nil {
		return 0
	}
	return int64(binary.NativeEndian.Uint64(raw))
}

func (h *managerHarness) walks(name string) int {
	h.resultsStore.mu.Lock()
	defer h.resultsStore.mu.Unlock()
	return h.resultsStore.sets[string(LastWalkTimeKey(name))]
}

func (h *managerHarness) results(name string) []string {
	raw, err := h.resultsStore.Get([]byte(name))
	require.NoError(h.t, err)
	var paths []string
	require.NoError(h.t, json.Unmarshal(raw, &paths))
	return relPaths(h.t, h.root, paths)
}

// relPaths prepends base to all provided relative paths and returns them.
func relPaths(t *testing.T, base string, paths []string) []string {
	t.Helper()
	rels := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(base, path)
		require.NoError(t, err)
		rels = append(rels, filepath.ToSlash(rel))
	}
	return rels
}
