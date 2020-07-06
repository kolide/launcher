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

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
)

// S_FALSE is returned by CoInitializeEx if it was already called on this thread.
const S_FALSE = 0x00000001

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

func Query(ctx context.Context, className string, properties []string, opts ...Option) ([]map[string]interface{}, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "wmi.Query")
	handler := NewOleHandler(ctx, properties)

	// settings
	qs := &querySettings{}
	for _, opt := range opts {
		opt(qs)
	}

	// If we query for the exact fields, _and_ one of the property
	// names is wrong, we get no results. (clearly an error. but I
	// can't find it) So query for `*`, and then fetch the
	// property. More testing might show this needs to change
	queryString := fmt.Sprintf("SELECT * FROM %s", className)

	// Initialize the COM system.
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		oleCode := err.(*ole.OleError).Code()
		if oleCode != ole.S_OK && oleCode != S_FALSE {
			return nil, errors.Wrap(err, "CoInitialize returned error")
		}
		level.Debug(logger).Log("msg", "The COM library is already initialized on this thread")
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, errors.Wrap(err, "ole createObject")
	}
	defer unknown.Release()

	wmi, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, errors.Wrap(err, "query interface create")
	}
	defer wmi.Release()

	// service is a SWbemServices
	serviceRaw, err := oleutil.CallMethod(wmi, "ConnectServer", qs.ConnectServerArgs()...)
	if err != nil {
		return nil, errors.Wrap(err, "wmi connectserver")
	}
	defer serviceRaw.Clear()

	service := serviceRaw.ToIDispatch()
	defer service.Release()

	// result is a SWBemObjectSet
	resultRaw, err := oleutil.CallMethod(service, "ExecQuery", queryString)
	if err != nil {
		return nil, errors.Wrapf(err, "Running query %s", queryString)
	}
	defer resultRaw.Clear()

	result := resultRaw.ToIDispatch()
	defer result.Release()

	if err := oleutil.ForEach(result, handler.HandleVariant); err != nil {
		return nil, errors.Wrap(err, "ole foreach")
	}

	return handler.results, nil
}

type oleHandler struct {
	logger     log.Logger
	results    []map[string]interface{}
	properties []string
}

func NewOleHandler(ctx context.Context, properties []string) *oleHandler {
	return &oleHandler{
		logger:     log.With(ctxlog.FromContext(ctx), "caller", "oleHandler"),
		properties: properties,
		results:    []map[string]interface{}{},
	}
}

func (oh *oleHandler) HandleVariant(v *ole.VARIANT) error {
	item := v.ToIDispatch()
	defer item.Release()

	result := make(map[string]interface{})

	for _, p := range oh.properties {
		val, err := oleutil.GetProperty(item, p)
		if err != nil {
			level.Debug(oh.logger).Log("msg", "Got error looking for property", "property", p, "err", err)
			continue
		}
		defer val.Clear()

		// Not sure if we need to special case the nil, or iv Value() handles it.
		if val.VT == 0x1 { //VT_NULL
			result[p] = nil
			continue
		}

		result[p] = val.Value()
	}
	if len(result) > 0 {
		oh.results = append(oh.results, result)
	}

	return nil
}
