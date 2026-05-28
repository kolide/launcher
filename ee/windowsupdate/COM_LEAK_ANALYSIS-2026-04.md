# COM Memory Leak Analysis: windowsupdate package

## Background

The `pkg/windows/windowsupdate` package provides a Go binding for the Windows
Update Agent API using go-ole. It derives from
https://github.com/ceshihao/windowsupdate (which has the same bugs).

A memory leak was observed when calling `IUpdateSearcher.Search` in-process.
The leak was severe enough that a workaround was added: run the query in a
subprocess (`launcher query-windowsupdates`) so the leaked memory is reclaimed
on process exit. See `ee/tables/windowsupdatetable/windowsupdate.go` and
`cmd/launcher/query_windowsupdates_windows.go`.

## Root Cause

The leak is caused by **missing COM reference cleanup** throughout the package.
There are two categories:

### 1. VARIANTs never cleared

Every call to `oleutil.GetProperty` or `oleutil.CallMethod` returns a
`*ole.VARIANT`. The VARIANT owns a reference to the underlying COM data (a
BSTR for strings, an IDispatch for objects, etc.). The caller must call
`result.Clear()` when done, which calls the Win32 `VariantClear` function to
free that reference.

The package used an `oleconv` helper layer (`pkg/windows/oleconv`) that
consumed the VARIANT, extracted the Go value, and returned -- but **never
called `Clear()`**. Since every property access on every COM object went through
oleconv, this leaked on every single `GetProperty` call. For a search returning
200 updates with ~40 properties each, that's ~8000+ leaked VARIANTs per query.

### 2. IDispatch pointers never Released

Every struct (`IUpdateSession`, `IUpdateSearcher`, `ISearchResult`, `IUpdate`,
`ICategory`, `IUpdateHistoryEntry`, etc.) stored a `disp *ole.IDispatch` field
but never called `Release()` on it. Additionally, intermediate IDispatch
pointers from collection iteration (e.g. the IUpdateCollection dispatch from
`GetProperty(searchResult, "Updates")`) were used and then abandoned.

### 3. IUnknown from CreateObject never Released

`oleutil.CreateObject` returns an `*ole.IUnknown` with refcount 1.
`QueryInterface(IID_IDispatch)` adds another reference. The original IUnknown
was never Released, leaking one reference per session creation. (This was fixed
in the first pass of this investigation.)

## What about STA vs MTA?

The go-comshim library initializes COM in MTA (Multi-Threaded Apartment) mode.
A note from a Windows developer suggested the leak might be related to
STA/MTA threading and IUnknown reference count bugs (off-by-one, "0 vs 1").

Investigation found that `Microsoft.Update.Session` (CLSID
`{4CB43D7F-7EEE-4906-8698-60DA1C38F2FE}`) is registered with
`ThreadingModel = "Both"`, meaning it works in either STA or MTA. So apartment
model mismatch is **not** the root cause for this specific COM class.

However, go-ole does have documented apartment-sensitive leak behavior
(https://github.com/go-ole/go-ole/issues/135), so apartment model can be a
contributing factor in general.

## The oleconv problem

`oleconv` (`pkg/windows/oleconv/oleconv.go`) was a thin wrapper that took a
`(*ole.VARIANT, error)` tuple from oleutil and returned a typed Go value. It
existed only to make property access a one-liner:

    oleconv.ToStringErr(oleutil.GetProperty(disp, "Title"))

The problem: it consumed the VARIANT and discarded it, making cleanup
impossible at the call site. The VARIANT couldn't be `Clear()`'d because it
was already lost. oleconv had no other consumers outside the windowsupdate
package.

## The fix: drop oleconv, follow wmi.go pattern

The `ee/wmi/wmi.go` package in this same codebase does COM correctly:
- `defer unknown.Release()` after CreateObject
- `defer disp.Release()` after QueryInterface
- `defer raw.Clear()` after GetProperty/CallMethod
- Does NOT call Release on an IDispatch obtained via ToIDispatch(), because
  ToIDispatch() is just a cast to the same memory; Clear() handles it

The fix rewrites the windowsupdate package to follow this same pattern:
- Work with raw `*ole.VARIANT` directly
- `defer variant.Clear()` immediately after receiving it
- Extract the Go value from the variant after deferring the clear
- `defer disp.Release()` in each `toI*` function for the dispatch being consumed
- Remove the `disp` field from structs (it's never used after construction
  in the query path)
- Delete oleconv entirely

## Key references

- COM refcount rules: https://learn.microsoft.com/en-us/windows/win32/com/rules-for-managing-reference-counts
- IUnknown: https://learn.microsoft.com/en-us/windows/win32/com/using-and-implementing-iunknown
- CoInitializeEx (STA/MTA): https://learn.microsoft.com/en-us/windows/win32/api/combaseapi/nf-combaseapi-coinitializeex
- go-ole issue #135 (apartment-sensitive leak): https://github.com/go-ole/go-ole/issues/135
- go-ole IUnknown: https://github.com/go-ole/go-ole/blob/master/iunknown.go
- Upstream library (same bugs): https://github.com/ceshihao/windowsupdate
- wmi.go arm64 panic note: ToIDispatch() is a cast, not a new reference;
  calling Release() on the IDispatch AND Clear() on the VARIANT double-frees
  and panics on arm64. Clear() the VARIANT only.
