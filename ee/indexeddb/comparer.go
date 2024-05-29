package indexeddb

import (
	"github.com/kolide/goleveldb/leveldb/comparer"
)

// chromeComparer is a thin wrapper around the default comparer to allow us
// to use it as `idb_cmp1`, which is the comparer used in Chromium's IndexedDB
// implementation.
type chromeComparer struct {
	defaultComparer comparer.Comparer
}

func newChromeComparer() *chromeComparer {
	return &chromeComparer{
		defaultComparer: comparer.DefaultComparer,
	}
}

func (c *chromeComparer) Compare(a, b []byte) int {
	return c.defaultComparer.Compare(a, b)
}

func (c *chromeComparer) Name() string {
	return "idb_cmp1"
}

func (c *chromeComparer) Separator(dst, a, b []byte) []byte {
	return c.defaultComparer.Separator(dst, a, b)
}

func (c *chromeComparer) Successor(dst, b []byte) []byte {
	return c.defaultComparer.Successor(dst, b)
}
