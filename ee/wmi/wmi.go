//go:build windows
// +build windows

// Package wmi provides a basic interface for querying against
// wmi. It's based on some underlying examples using ole [1].
//
// We do _not_ use the stackdriver library [2], because that uses reflect
// and wants typed objects. Our use case is too dynamic.
//
// To understand the available classes, take a look at the Microsoft
// documention [3]
//
// Servers, Namespaces, and connection parameters:
//
// WMI has a fairly rich set of connection options. It allows querying
// on remote servers, via authenticated users names, in different name
// spaces... These options are exposed through functional arguments.
//
// References:
//
// 1. https://stackoverflow.com/questions/20365286/query-wmi-from-go
// 2. https://github.com/StackExchange/wmi
// 3. https://docs.microsoft.com/en-us/windows/win32/cimwin32prov/operating-system-classes
//
// Namespaces, ongoing:
//
// To list them: gwmi -namespace "root" -class "__Namespace" | Select Name
// To list classes: gwmi -namespace root\cimv2 -list
// Default: ROOT\CIMV2
//
// Get-WmiObject -Query "select * from win32_service where name='WinRM'"
// Get-WmiObject  -namespace root\cimv2\security\MicrosoftTpm -Query "SELECT * FROM Win32_Tpm"
package wmi

import (
	"context"
	"fmt"
	"log/slog"

	comshim "github.com/NozomiNetworks/go-comshim"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

const (
	S_FALSE                        = 0x00000001 // S_FALSE is returned by CoInitializeEx if it was already called on this thread.
	WBEM_FLAG_RETURN_WHEN_COMPLETE = 0          // https://learn.microsoft.com/en-us/windows/win32/wmisdk/swbemservices-execquery#parameters
	WBEM_FLAG_FORWARD_ONLY         = 32
)

// querySettings contains various options. Mostly for the
// connectServerArgs args. See
// https://docs.microsoft.com/en-us/windows/win32/wmisdk/swbemlocator-connectserver
// for details.
type querySettings struct {
	connectServer        string
	connectNamespace     string
	connectUser          string
	connectPassword      string
	connectLocale        string
	connectAuthority     string
	connectSecurityFlags uint
	whereClause          string
}

// ConnectServerArgs returns an array suitable for being passed to ole
// call ConnectServer
func (qs *querySettings) ConnectServerArgs() []interface{} {
	return []interface{}{
		qs.connectServer,
		qs.connectNamespace,
		qs.connectUser,
		qs.connectPassword,
		qs.connectLocale,
		qs.connectAuthority,
		qs.connectSecurityFlags,
	}
}

type Option func(*querySettings)

// ConnectServer sets the server to connect to. It defaults to "",
// which is localhost.
func ConnectServer(s string) Option {
	return func(qs *querySettings) {
		qs.connectServer = s
	}
}

// ConnectNamespace sets the namespace to query against. It defaults
// to "", which is the same as `ROOT\CIMV2`
func ConnectNamespace(s string) Option {
	return func(qs *querySettings) {
		qs.connectNamespace = s
	}
}

// ConnectUseMaxWait requires that ConnectServer use a timeout. The
// call is then guaranteed to return in 2 minutes or less. This option
// is strongly recommended, as without it calls can block forever.
func ConnectUseMaxWait() Option {
	return func(qs *querySettings) {
		// see the definition of iSecurityFlags in
		// https://docs.microsoft.com/en-us/windows/win32/wmisdk/swbemlocator-connectserver
		qs.connectSecurityFlags = qs.connectSecurityFlags & 128
	}
}

// WithWhere will be used for the optional WHERE clause in wmi.
func WithWhere(whereClause string) Option {
	return func(qs *querySettings) {
		qs.whereClause = whereClause
	}
}

func Query(ctx context.Context, slogger *slog.Logger, className string, properties []string, opts ...Option) ([]map[string]interface{}, error) {
	handler := NewOleHandler(ctx, slogger, properties)

	// settings
	qs := &querySettings{}
	for _, opt := range opts {
		opt(qs)
	}

	var whereClause string
	if qs.whereClause != "" {
		whereClause = fmt.Sprintf(" WHERE %s", qs.whereClause)
	}

	// If we query for the exact fields, _and_ one of the property
	// names is wrong, we get no results. (clearly an error. but I
	// can't find it) So query for `*`, and then fetch the
	// property. More testing might show this needs to change
	queryString := fmt.Sprintf("SELECT * FROM %s%s", className, whereClause)

	// Initialize the COM system.
	if err := comshim.TryAdd(1); err != nil {
		comshim.Done() // ensure we decrement the global shim counter that TryAdd increments immediately
		return nil, fmt.Errorf("unable to init comshim: %w", err)
	}
	defer comshim.Done()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, fmt.Errorf("ole createObject: %w", err)
	}
	defer unknown.Release()

	wmi, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("query interface create: %w", err)
	}
	defer wmi.Release()

	// service is a SWbemServices
	serviceRaw, err := oleutil.CallMethod(wmi, "ConnectServer", qs.ConnectServerArgs()...)
	if err != nil {
		return nil, fmt.Errorf("wmi connectserver: %w", err)
	}
	defer serviceRaw.Clear()

	// In testing, we find we do not need to `service.Release()`. The memory of result is released
	// by `defer serviceRaw.Clear()` above, furthermore on windows arm64 machines, calling
	// `service.Clear()` after `serviceRaw.Release()` causes a panic.
	//
	// Looking at the `serviceRaw.ToIDispatch()` implementation, it's just a cast that returns
	// a pointer to the same memory. Which would explain why calling `serviceRaw.Release()` after
	// `service.Clear()` causes a panic. It's unclear why this causes a panic on arm64 machines and
	// not on amd64 machines.
	//
	// This also applies to the `resultRaw` and `results` variables below.
	service := serviceRaw.ToIDispatch()

	slogger.Log(ctx, slog.LevelDebug,
		"running WMI query",
		"query", queryString,
	)

	// ExecQuery runs semi-synchronously by default. To ensure we aren't missing any results,
	// we prefer synchronous mode, which we achieve by setting iFlags to wbemFlagForwardOnly+wbemFlagReturnWhenComplete
	// instead of the default wbemFlagReturnImmediately. (wbemFlagReturnWhenComplete will make the call synchronous,
	// and wbemFlagForwardOnly helps us avoid any potential performance issues.) The flags values are not
	// incredibly well-documented and there are multiple zero-value flags. We assume that wbemFlagForwardOnly (32)
	// and wbemFlagBidirectional (0) are mutually exclusive, and that wbemFlagReturnImmediately (16) and
	// wbemFlagReturnWhenComplete (0) are mutually exclusive. We assume, therefore, that WMI correctly understands
	// an `iFlags` value of 32 as wbemFlagForwardOnly+wbemFlagReturnWhenComplete. (It cannot be understood as
	// wbemFlagForwardOnly+wbemFlagBidirectional, because that is not a possible combination. It cannot be understood
	// as wbemFlagForwardOnly only, because the return behavior flag must be set as either wbemFlagReturnWhenComplete or
	// wbemFlagReturnImmediately.)
	// See
	// * https://learn.microsoft.com/en-us/windows/win32/wmisdk/calling-a-method#semisynchronous-mode.
	// * https://learn.microsoft.com/en-us/windows/win32/wmisdk/swbemservices-execquery#parameters
	// The result is a SWBemObjectSet.
	resultRaw, err := oleutil.CallMethod(service, "ExecQuery", queryString, "WQL", WBEM_FLAG_FORWARD_ONLY+WBEM_FLAG_RETURN_WHEN_COMPLETE)
	if err != nil {
		return nil, fmt.Errorf("running query `%s`: %w", queryString, err)
	}
	defer resultRaw.Clear()

	// see above comment about `service.Release()` to explain why `result.Release()` isn't called
	result := resultRaw.ToIDispatch()

	if err := oleutil.ForEach(result, handler.HandleVariant); err != nil {
		return nil, fmt.Errorf("ole foreach: %w", err)
	}

	return handler.results, nil
}

type oleHandler struct {
	slogger    *slog.Logger
	results    []map[string]interface{}
	properties []string
}

func NewOleHandler(ctx context.Context, slogger *slog.Logger, properties []string) *oleHandler {
	return &oleHandler{
		slogger:    slogger.With("component", "ole_handler"),
		properties: properties,
		results:    []map[string]interface{}{},
	}
}

func (oh *oleHandler) HandleVariant(v *ole.VARIANT) error {
	item := v.ToIDispatch()
	defer item.Release()

	result := make(map[string]interface{})

	for _, p := range oh.properties {
		prop, err := getProperty(item, p)
		if err != nil {
			oh.slogger.Log(context.TODO(), slog.LevelDebug,
				"got error looking for property",
				"property", p,
				"err", err,
			)
			continue
		}
		result[p] = prop

	}
	if len(result) > 0 {
		oh.results = append(oh.results, result)
	}

	return nil
}

func getProperty(item *ole.IDispatch, property string) (any, error) {
	val, err := oleutil.GetProperty(item, property)
	if err != nil {
		return nil, fmt.Errorf("looking for property %s: %w", property, err)
	}
	defer val.Clear()

	// Not sure if we need to special case the nil, or if Value() handles it.
	if val.VT == 0x1 { //VT_NULL
		return nil, nil
	}

	// Attempt to handle arrays
	safeArray := val.ToArray()
	if safeArray != nil {
		// I would have expected to need
		// `defersafeArray.Release()` here, if I add
		// that, this routine stops working.
		return safeArray.ToValueArray(), nil
	}

	return val.Value(), nil
}
