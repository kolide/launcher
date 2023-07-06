//go:build windows
// +build windows

package main

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
	}
)

const (
	eventIdEnteringModernStandby = 506
	eventIdExitingModernStandby  = 507
	eventIdEnteringSleep         = 42

	operationSuccessfulMsg = "The operation completed successfully."
)

func newPowerEventWatcher(logger log.Logger) *powerEventWatcher {
	evtApi := syscall.NewLazyDLL("wevtapi.dll")

	return &powerEventWatcher{
		logger:                  logger,
		subscribeProcedure:      evtApi.NewProc("EvtSubscribe"),
		unsubscribeProcedure:    evtApi.NewProc("EvtClose"),
		renderEventLogProcedure: evtApi.NewProc("EvtRender"),
	}
}

// subscribeToPowerEvents sets up a subscription to relevant power events with a callback to `onPowerEvent`.
func (p *powerEventWatcher) subscribeToPowerEvents() {
	// WINEVENT_CHANNEL_GLOBAL_SYSTEM is "System"
	channelPath, err := syscall.UTF16PtrFromString("System")
	if err != nil {
		level.Debug(p.logger).Log("msg", "error creating pointer to channel path", "err", err)
		return
	}

	queryStr := fmt.Sprintf("*[System[Provider[@Name='Microsoft-Windows-Kernel-Power'] and (EventID=%d or EventID=%d or EventID=%d)]]",
		eventIdEnteringModernStandby,
		eventIdExitingModernStandby,
		eventIdEnteringSleep,
	)
	query, err := syscall.UTF16PtrFromString(queryStr)
	if err != nil {
		level.Debug(p.logger).Log("msg", "error creating pointer to query", "err", err)
		return
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
		level.Debug(p.logger).Log("msg", "error subscribing to future power events", "last_err", err)
	}

	// Save the handle so that we can close it later
	p.subscriptionHandle = subscriptionHandle
}

func (p *powerEventWatcher) shutdown() {
	// EvtClose: https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtclose
	ret, _, err := p.unsubscribeProcedure.Call(p.subscriptionHandle)
	level.Debug(p.logger).Log("msg", "unsubscribed from power events", "ret", fmt.Sprintf("%+v", ret), "last_err", err)
}

// onPowerEvent implements EVT_SUBSCRIBE_CALLBACK -- see https://learn.microsoft.com/en-us/windows/win32/api/winevt/nc-winevt-evt_subscribe_callback
func (p *powerEventWatcher) onPowerEvent(action uint32, _ uintptr, eventHandle uintptr) (ret uintptr) {
	if action == 0 {
		level.Debug(p.logger).Log("msg", "received EvtSubscribeActionError when watching power events", "err_code", uint32(eventHandle))
		return
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
		return
	}

	// Prevent us from indexing beyond the size of the array.
	// This shouldn't happen? I did definitely see it in test at least once, where I'd allocated 3000 bytes
	// but it reported a BufferUsed of over 5000.
	if bufUsed > uint32(bufferSize) {
		bufUsed = uint32(bufferSize)
	}

	buf = buf[:bufUsed-1]

	// The returned XML string is UTF-16-encoded, so we decode it here before parsing the XML.
	decoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
	utf8bytes, err := decoder.Bytes(buf)
	if err != nil {
		level.Debug(p.logger).Log("msg", "error decoding from utf16 to utf8", "err", err)
		return
	}

	var e eventLogEntry
	if err := xml.Unmarshal(utf8bytes, &e); err != nil {
		level.Debug(p.logger).Log("msg", "error unmarshalling event log entry", "err", err)
		return
	}

	if e.System.EventID == eventIdEnteringModernStandby || e.System.EventID == eventIdEnteringSleep {
		level.Debug(p.logger).Log("msg", "system is sleeping")
		// TODO -- do something! Make launcher stop operations? Make launcher ignore errors that would otherwise
		// make it restart itself?
	} else if e.System.EventID == eventIdExitingModernStandby {
		level.Debug(p.logger).Log("msg", "system is waking")
		// TODO -- wake launcher back up too
	} else {
		level.Debug(p.logger).Log("msg", "received unexpected event ID in log", "event_id", e.System.EventID, "raw_event", string(utf8bytes))
	}

	return
}
