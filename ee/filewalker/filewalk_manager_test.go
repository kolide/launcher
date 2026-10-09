package filewalker

import (
	"encoding/binary"
	"encoding/json"
	"os"
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

		walks := map[string]int{}
		previous := map[string]int64{"dotenv": h.lastWalk("dotenv"), "host_keys": h.lastWalk("host_keys")}
		for range int(8 * time.Hour / walkCheckInterval) {
			h.advance(walkCheckInterval)
			for name, last := range previous {
				if current := h.lastWalk(name); current != last {
					walks[name]++
					previous[name] = current
				}
			}
		}

		require.Equal(t, map[string]int{"dotenv": 4, "host_keys": 2}, walks)
	})
}

// Ping should walk every config on the next tick, even those not due; ticks without a Ping should not.
func TestManager_PingWalksAllOnNextTick(t *testing.T) {
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
		h.advance(walkCheckInterval)
		require.Equal(t, time.Now().Unix(), h.lastWalk("dotenv"))
		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
	})
}

// Several Pings before a tick should produce a single full walk.
func TestManager_PingsCoalesce(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.start()

		h.fm.Ping()
		h.fm.Ping()
		h.fm.Ping()
		h.advance(walkCheckInterval)
		pingWalk := time.Now().Unix()
		require.Equal(t, pingWalk, h.lastWalk("dotenv"))

		h.advance(walkCheckInterval)
		require.Equal(t, pingWalk, h.lastWalk("dotenv"))
	})
}

func TestManager_DoWalksNamedImmediately(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()
		bootWalk := time.Now().Unix()

		h.advance(time.Minute)
		require.NoError(t, h.do(`{"filewalks": ["host_keys"]}`))
		synctest.Wait()

		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
		require.Equal(t, bootWalk, h.lastWalk("dotenv"))
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

		require.Equal(t, time.Now().Unix(), h.lastWalk("dotenv"))
		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
	})
}

func TestManager_DoIgnoresUnknownNames(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("host_keys", hostKeysCfg)
		h.start()

		h.advance(time.Minute)
		require.NoError(t, h.do(`{"filewalks": ["missing", "host_keys"]}`))
		synctest.Wait()

		require.Equal(t, time.Now().Unix(), h.lastWalk("host_keys"))
		require.Zero(t, h.lastWalk("missing"))
	})
}

// Do may reject a request while another is queued; retried requests, as the actionqueue does, should all walk.
func TestManager_DoRetriedUntilAccepted(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.setConfig("host_keys", hostKeysCfg)
		h.start()

		for _, req := range []string{
			`{"filewalks": ["dotenv"]}`,
			`{"filewalks": ["host_keys"]}`,
			`{"filewalks": ["dotenv", "host_keys"]}`,
			`{"filewalks": ["host_keys"]}`,
		} {
			h.advance(time.Minute)
			for h.do(req) != nil {
				synctest.Wait()
			}
			synctest.Wait()

			for _, name := range []string{"dotenv", "host_keys"} {
				if strings.Contains(req, name) {
					require.Equal(t, time.Now().Unix(), h.lastWalk(name), req)
				}
			}
		}
	})
}

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

		h.setConfig("host_keys", `{not json`)
		h.fm.Ping()
		h.advance(walkCheckInterval)

		require.Equal(t, time.Now().Unix(), h.lastWalk("dotenv"))
		require.Equal(t, bootWalk, h.lastWalk("host_keys"))
		require.Equal(t, []string{"etc/ssh/ssh_host_ed25519_key"}, h.results("host_keys"))
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

func TestManager_InterruptBeforeExecute(t *testing.T) {
	t.Parallel()
	runManagerTest(t, func(h *managerHarness) {
		h.setConfig("dotenv", dotenvCfg)
		h.fm.Interrupt(nil)

		h.start()

		require.True(t, h.stopped())
	})
}

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

	require.Equal(t, []string{"elapsed", "future", "never_walked"}, due(now, cfgs, lastWalks))
}

type managerHarness struct {
	t            *testing.T
	root         string
	cfgStore     types.KVStore
	resultsStore types.KVStore
	fm           *FilewalkManager
	done         chan struct{}
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
		resultsStore, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
		require.NoError(t, err)
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
	require.NoError(h.t, h.resultsStore.Set(LastWalkTimeKey(name), binary.NativeEndian.AppendUint64(nil, uint64(ts))))
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

func (h *managerHarness) results(name string) []string {
	raw, err := h.resultsStore.Get([]byte(name))
	require.NoError(h.t, err)
	var paths []string
	require.NoError(h.t, json.Unmarshal(raw, &paths))
	return relPaths(h.t, h.root, paths)
}
