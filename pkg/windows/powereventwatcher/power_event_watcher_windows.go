//go:build windows
// +build windows

package powereventwatcher

import (
	"encoding/xml"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
		logger                  log.Logger
		subscriptionHandle      uintptr
		subscribeProcedure      *syscall.LazyProc
		unsubscribeProcedure    *syscall.LazyProc
		renderEventLogProcedure *syscall.LazyProc
		interrupt               chan struct{}
	}
)

const (
	eventIdEnteringModernStandby = 506
	eventIdExitingModernStandby  = 507
	eventIdEnteringSleep         = 42

	operationSuccessfulMsg = "The operation completed successfully."
)

// New sets up a subscription to relevant power events with a callback to `onPowerEvent`.
func New(logger log.Logger) (*powerEventWatcher, error) {
	evtApi := syscall.NewLazyDLL("wevtapi.dll")

	p := &powerEventWatcher{
		logger:                  logger,
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

	queryStr := fmt.Sprintf("*[System[Provider[@Name='Microsoft-Windows-Kernel-Power'] and (EventID=%d or EventID=%d or EventID=%d)]]",
		eventIdEnteringModernStandby,
		eventIdExitingModernStandby,
		eventIdEnteringSleep,
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

	return p, nil
}

// Execute is a no-op, since we've already registered our subscription
func (p *powerEventWatcher) Execute() error {
	<-p.interrupt
	return nil
}

func (p *powerEventWatcher) Interrupt(_ error) {
	// EvtClose: https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
	ret, _, err := p.unsubscribeProcedure.Call(p.subscriptionHandle)
	level.Debug(p.logger).Log("msg", "unsubscribed from power events", "ret", fmt.Sprintf("%+v", ret), "last_err", err)

	p.interrupt <- struct{}{}
}

// onPowerEvent implements EVT_SUBSCRIBE_CALLBACK -- see https://learn.microsoft.com/en-us/windows/win32/api/winevt/nc-winevt-evt_subscribe_callback
func (p *powerEventWatcher) onPowerEvent(action uint32, _ uintptr, eventHandle uintptr) uintptr {
	var ret uintptr // We never do anything with this and neither does Windows -- it's here to satisfy the interface
	if action == 0 {
		level.Debug(p.logger).Log("msg", "received EvtSubscribeActionError when watching power events", "err_code", uint32(eventHandle))
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
		level.Debug(p.logger).Log("msg", "error calling EvtRender to get event details", "last_err", err)
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
		level.Debug(p.logger).Log("msg", "error decoding from utf16 to utf8", "err", err)
		return ret
	}

	var e eventLogEntry
	if err := xml.Unmarshal(utf8bytes, &e); err != nil {
		level.Debug(p.logger).Log("msg", "error unmarshalling event log entry", "err", err)
		return ret
	}

	switch e.System.EventID {
	case eventIdEnteringModernStandby, eventIdEnteringSleep:
		level.Debug(p.logger).Log("msg", "system is sleeping", "event_id", e.System.EventID)
	case eventIdExitingModernStandby:
		level.Debug(p.logger).Log("msg", "system is waking", "event_id", e.System.EventID)
	default:
		level.Debug(p.logger).Log("msg", "received unexpected event ID in log", "event_id", e.System.EventID, "raw_event", string(utf8bytes))
	}

	return ret
}
