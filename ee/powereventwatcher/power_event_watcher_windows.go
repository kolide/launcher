//go:build windows
// +build windows

package powereventwatcher

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"golang.org/x/text/encoding/unicode"
)

type (
	eventLogEntry struct {
		XMLName xml.Name `xml:"Event"`
		System  System   `xml:"System"`
	}

	System struct {
		EventID int `xml:"EventID"`
	}

	powerEventWatcher struct {
		slogger                 *slog.Logger
		powerEventSubscriber    powerEventSubscriber
		subscriptionHandle      uintptr
		subscribeProcedure      *syscall.LazyProc
		unsubscribeProcedure    *syscall.LazyProc
		renderEventLogProcedure *syscall.LazyProc
		interrupt               chan struct{}
		interrupted             atomic.Bool
	}

	// powerEventSubscriber is an interface to be implemented by anything utilizing the power event updates.
	// implementers are provided to New, and the interface methods below are called as described during relevant updates
	powerEventSubscriber interface {
		// OnPowerEvent will be called for the provided subscriber whenever any watched event is observed
		OnPowerEvent(eventID int) error
		// OnStartup will be called when the powerEventWatcher is initially set up, allowing subscribers
		// to perform any setup behavior (e.g. cache clearing, state resetting)
		OnStartup() error
	}

	// knapsackSleepStateUpdater implements the powerEventSubscriber interface and
	// updates the knapsack.InModernStandby state based on the power events observed
	knapsackSleepStateUpdater struct {
		knapsack types.Knapsack
		slogger  *slog.Logger
	}

	// InMemorySleepStateUpdater implements the powerEventSubscriber interface. When passed as
	// the powerEventSubscriber, it will expose the last seen ModernStandby state for the caller
	InMemorySleepStateUpdater struct {
		sync.Mutex
		slogger         *slog.Logger
		inModernStandby bool
	}
)

const (
	eventIdEnteringModernStandby = 506
	EventIdExitingModernStandby  = 507
	eventIdEnteringSleep         = 42
	EventIdResumedFromSleep      = 107

	operationSuccessfulMsg = "The operation completed successfully."
)

func NewKnapsackSleepStateUpdater(slogger *slog.Logger, k types.Knapsack) *knapsackSleepStateUpdater {
	return &knapsackSleepStateUpdater{
		knapsack: k,
		slogger:  slogger,
	}
}

func NewInMemorySleepStateUpdater(slogger *slog.Logger) *InMemorySleepStateUpdater {
	return &InMemorySleepStateUpdater{
		slogger: slogger,
	}
}

func (ims *InMemorySleepStateUpdater) OnStartup() error {
	ims.Lock()
	defer ims.Unlock()
	// this should essentially be a no-op for our inmemory store since it will default false
	ims.inModernStandby = false
	return nil
}

func (ims *InMemorySleepStateUpdater) InModernStandby() bool {
	ims.Lock()
	defer ims.Unlock()

	return ims.inModernStandby
}

func (ims *InMemorySleepStateUpdater) OnPowerEvent(eventID int) error {
	ims.Lock()
	defer ims.Unlock()

	switch eventID {
	case eventIdEnteringModernStandby, eventIdEnteringSleep:
		ims.inModernStandby = true
	case EventIdExitingModernStandby, EventIdResumedFromSleep:
		ims.inModernStandby = false
	default:
		ims.slogger.Log(context.TODO(), slog.LevelWarn,
			"received unexpected event ID in log",
			"event_id", eventID,
		)
	}

	return nil
}

func (ks *knapsackSleepStateUpdater) OnPowerEvent(eventID int) error {
	switch eventID {
	case eventIdEnteringModernStandby, eventIdEnteringSleep:
		ks.slogger.Log(context.TODO(), slog.LevelDebug,
			"system is sleeping",
			"event_id", eventID,
		)
		if err := ks.knapsack.SetInModernStandby(true); err != nil {
			ks.slogger.Log(context.TODO(), slog.LevelWarn,
				"encountered error setting modern standby value",
				"in_modern_standby", true,
				"err", err,
			)
		}
	case EventIdExitingModernStandby, EventIdResumedFromSleep:
		ks.slogger.Log(context.TODO(), slog.LevelDebug,
			"system is waking",
			"event_id", eventID,
		)
		if err := ks.knapsack.SetInModernStandby(false); err != nil {
			ks.slogger.Log(context.TODO(), slog.LevelWarn,
				"encountered error setting modern standby value",
				"in_modern_standby", false,
				"err", err,
			)
		}
	default:
		ks.slogger.Log(context.TODO(), slog.LevelWarn,
			"received unexpected event ID in log",
			"event_id", eventID,
		)
	}

	return nil
}

func (ks *knapsackSleepStateUpdater) OnStartup() error {
	// Clear InModernStandby flag, in case it's cached. We may have missed wake/sleep events
	// while launcher was not running, and we want to err on the side of assuming the device
	// is awake.
	return ks.knapsack.SetInModernStandby(false)
}

// New sets up a subscription to relevant power events with a callback to `onPowerEvent`.
func New(ctx context.Context, slogger *slog.Logger, pes powerEventSubscriber) (*powerEventWatcher, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	evtApi := syscall.NewLazyDLL("wevtapi.dll")

	p := &powerEventWatcher{
		slogger:                 slogger.With("component", "power_event_watcher"),
		powerEventSubscriber:    pes,
		subscribeProcedure:      evtApi.NewProc("EvtSubscribe"),
		unsubscribeProcedure:    evtApi.NewProc("EvtClose"),
		renderEventLogProcedure: evtApi.NewProc("EvtRender"),
		interrupt:               make(chan struct{}),
	}

	// WINEVENT_CHANNEL_GLOBAL_SYSTEM is "System"
	channelPath, err := syscall.UTF16PtrFromString("System")
	if err != nil {
		return nil, fmt.Errorf("could not create pointer to channel path: %w", err)
	}

	queryStr := fmt.Sprintf("*[System[Provider[@Name='Microsoft-Windows-Kernel-Power'] and (EventID=%d or EventID=%d or EventID=%d or EventID=%d)]]",
		eventIdEnteringModernStandby,
		EventIdExitingModernStandby,
		eventIdEnteringSleep,
		EventIdResumedFromSleep,
	)
	query, err := syscall.UTF16PtrFromString(queryStr)
	if err != nil {
		return nil, fmt.Errorf("could not create pointer to query: %w", err)
	}

	// EvtSubscribe: https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtsubscribe
	// Flags: https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_subscribe_flags
	subscriptionHandle, _, err := p.subscribeProcedure.Call(
		0,                                    // Session -- NULL because we're querying the local computer
		0,                                    // SignalEvent -- NULL because we're setting a callback
		uintptr(unsafe.Pointer(channelPath)), // ChannelPath -- the channel in the event log
		uintptr(unsafe.Pointer(query)),       // Query -- our event filter
		0,                                    // Bookmark -- NULL because we're only subscribing to future events
		0,                                    // Context -- can be used to pass info to the callback, but we don't need to do that
		syscall.NewCallback(p.onPowerEvent),  // Callback -- executed every time an event matching our query occurs
		uintptr(uint32(1)),                   // Flags -- EvtSubscribeToFutureEvents has value 1
	)
	if err != nil && err.Error() != operationSuccessfulMsg {
		return nil, fmt.Errorf("could not subscribe to future power events: %w", err)
	}

	// Save the handle so that we can close it later
	p.subscriptionHandle = subscriptionHandle

	if err := p.powerEventSubscriber.OnStartup(); err != nil {
		// log any issues here but don't prevent creation of the watcher
		slogger.Log(ctx, slog.LevelError,
			"encountered error issuing subscriber OnStartup",
			"err", err,
		)
	}

	return p, nil
}

// Execute is a no-op, since we've already registered our subscription
func (p *powerEventWatcher) Execute() error {
	<-p.interrupt
	return nil
}

func (p *powerEventWatcher) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if p.interrupted.Swap(true) {
		return
	}

	// EvtClose: https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
	ret, _, err := p.unsubscribeProcedure.Call(p.subscriptionHandle)

	p.slogger.Log(context.TODO(), slog.LevelDebug,
		"unsubscribed from power events",
		"ret", fmt.Sprintf("%+v", ret),
		"last_err", err,
	)

	p.interrupt <- struct{}{}
}

// onPowerEvent implements EVT_SUBSCRIBE_CALLBACK -- see https://learn.microsoft.com/en-us/windows/win32/api/winevt/nc-winevt-evt_subscribe_callback
func (p *powerEventWatcher) onPowerEvent(action uint32, _ uintptr, eventHandle uintptr) uintptr {
	var ret uintptr // We never do anything with this and neither does Windows -- it's here to satisfy the interface
	if action == 0 {
		p.slogger.Log(context.TODO(), slog.LevelWarn,
			"received EvtSubscribeActionError when watching power events",
			"err_code", uint32(eventHandle),
		)
		return ret
	}

	// We've been delivered an event! Call EvtRender to get the details of that event, using the eventHandle.
	// EvtRender: https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
	// Flags: https://learn.microsoft.com/en-us/windows/win32/api/winevt/ne-winevt-evt_render_flags
	bufferSize := 10000
	buf := make([]byte, bufferSize)
	var bufUsed uint32
	var propertyCount uint32
	_, _, err := p.renderEventLogProcedure.Call(
		0,                                       // Context -- unused
		eventHandle,                             // Fragment -- the event handle
		uintptr(uint32(1)),                      // Flags -- EvtRenderEventXml has value 1
		uintptr(bufferSize),                     // BufferSize
		uintptr(unsafe.Pointer(&buf[0])),        // Buffer -- caller-allocated buffer to receive output
		uintptr(unsafe.Pointer(&bufUsed)),       // BufferUsed -- modified by call: the size, in bytes, of buffer used
		uintptr(unsafe.Pointer(&propertyCount)), // PropertyCount -- modified by call: only matters if we used EvtRenderEventValues
	)
	if err != nil && err.Error() != operationSuccessfulMsg {
		p.slogger.Log(context.TODO(), slog.LevelWarn,
			"error calling EvtRender to get event details",
			"last_err", err,
		)
		return ret
	}

	// Prevent us from indexing beyond the size of the array -- this seems like it should not
	// happen, given that we pass in bufferSize, but more than once saw a return value for bufUsed
	// that was greater than bufferSize.
	if bufUsed > uint32(bufferSize) {
		bufUsed = uint32(bufferSize)
	}

	buf = buf[:bufUsed-1]

	// The returned XML string is UTF-16-encoded, so we decode it here before parsing the XML.
	decoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
	utf8bytes, err := decoder.Bytes(buf)
	if err != nil {
		p.slogger.Log(context.TODO(), slog.LevelWarn,
			"error decoding from utf16 to utf8",
			"err", err,
		)
		return ret
	}

	var e eventLogEntry
	if err := xml.Unmarshal(utf8bytes, &e); err != nil {
		p.slogger.Log(context.TODO(), slog.LevelWarn,
			"error unmarshalling event log entry",
			"err", err,
		)
		return ret
	}

	if err := p.powerEventSubscriber.OnPowerEvent(e.System.EventID); err != nil {
		p.slogger.Log(context.TODO(), slog.LevelWarn,
			"subscriber encountered error OnPowerEvent update",
			"err", err,
		)
	}

	return ret
}
