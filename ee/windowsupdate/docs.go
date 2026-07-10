// Package windowsupdate provides a go-ole interface to the windows
// update agent.
//
// This code derives from https://github.com/ceshihao/windowsupdate
//
// # COM lifecycle management
//
// This package follows the same COM cleanup pattern as ee/wmi/wmi.go:
//
//   - oleutil.GetProperty / CallMethod return a *ole.VARIANT. We call
//     defer v.Clear() immediately to free the underlying COM reference
//     (BSTR, IDispatch, etc.) via Win32 VariantClear.
//   - For IDispatch results, we do NOT clear the VARIANT because
//     ToIDispatch() is a pointer cast into the same memory. Instead,
//     the caller calls Release() on the IDispatch when done. See the
//     arm64 panic note in ee/wmi/wmi.go for why this matters.
//   - CreateObject returns an IUnknown with refcount 1. QueryInterface
//     adds another ref. We defer unknown.Release() immediately.
//   - Each toI* function receives an IDispatch and (for types that
//     are fully consumed during construction) calls defer disp.Release().
//
// The helper functions in olehelpers.go centralize the VARIANT
// extraction + Clear() pattern so individual files don't need to
// repeat it.
//
// For background on the memory leak this addresses, see
// COM_LEAK_ANALYSIS.md in this directory.
package windowsupdate
