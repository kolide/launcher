// +build windows

// Package wmi provides a basic interface for querying against
// wmi. It's based on some underlying examples using ole [1].
//
// We do _not_ use the stackdriver library [2], because that uses reflect
// and wants typed objects. Our use case is too dynamic.
//
// References:
//
// 1. https://stackoverflow.com/questions/20365286/query-wmi-from-go
// 2. https://github.com/StackExchange/wmi
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

func Query(ctx context.Context, className string, properties []string) ([]map[string]interface{}, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "wmi.Query")
	handler := NewOleHandler(ctx, properties)

	// If we query for the exact fields, _and_ one of the property
	// names is wrong, we get no results. (clearly an error. but I
	// can't find it) So query for `*`, and then fetch the
	// property. More testing might show this needs to change
	queryString := fmt.Sprintf("SELECT * FROM %s", className)

	// Initialize the COM system.
	// This is using a single threaded model. See Docs.
	if err := ole.CoInitialize(0); err != nil {
		code := err.(*ole.OleError).Code()
		if code != 0x00000001 {
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
	serviceRaw, err := oleutil.CallMethod(wmi, "ConnectServer")
	if err != nil {
		return nil, errors.Wrap(err, "wmi connectserver")
	}
	service := serviceRaw.ToIDispatch()
	defer service.Release()

	// result is a SWBemObjectSet
	resultRaw, err := oleutil.CallMethod(service, "ExecQuery", queryString)
	if err != nil {
		return nil, errors.Wrapf(err, "Running query %s", queryString)
	}
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
			//continue
		} else {
			result[p] = val.ToString()
		}
	}
	if len(result) > 0 {
		oh.results = append(oh.results, result)
	}

	return nil
}
