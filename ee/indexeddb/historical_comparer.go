package indexeddb

import (
	"github.com/kolide/goleveldb/leveldb/comparer"
)

// historicalComparer is a wrapper around the default bytewise comparer to allow us
// return previous versions of keys, and keys that would otherwise be skipped based on
// their insertion/sort orders. It does this by overridding the Compare method to
// to return a positive result that would otherwise be skipped by the bytewise comparer.
// This is a bit of a hack, because it is unclear why the insertion order used by some of
// the leveldbs we're seeing is causing keys to be skipped. But it will allow cloud to see
// all active keys in the database for these cases, and perform analysis and filtering as needed.
type historicalComparer struct {
	bytewiseComparer comparer.Comparer
}

func newHistoricalComparer() *historicalComparer {
	return &historicalComparer{
		bytewiseComparer: comparer.DefaultComparer,
	}
}

// Compare defers to the default bytewaise comparer logic, but return a positive result if the compared keys are equal
// (would return 0). This causes historical versions of the same key to be yielded by the iterator, and prevents keys
// from being filtered out if they were inserted in a different order than would be yielded (based on key comparisons).
func (hc *historicalComparer) Compare(a, b []byte) int {
	ret := hc.bytewiseComparer.Compare(a, b)
	if ret == 0 {
		return 1
	}

	return ret
}

// Name defers to the default bytewise comparer name ("leveldb.BytewiseComparator") because
// for the leveldbs that we've seen this aggressive filtering issue for so far, that is the
// comparer required by the manifest.
func (hc *historicalComparer) Name() string {
	return hc.bytewiseComparer.Name()
}

func (hc *historicalComparer) Separator(dst, a, b []byte) []byte {
	return hc.bytewiseComparer.Separator(dst, a, b)
}

func (hc *historicalComparer) Successor(dst, b []byte) []byte {
	return hc.bytewiseComparer.Successor(dst, b)
}
