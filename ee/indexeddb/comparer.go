package indexeddb

import "bytes"

// chromeComparer implements comparer.Comparer interface for `idb_cmp1`
// this is just a bytewise comparer
type chromeComparer struct {
}

func (c *chromeComparer) Compare(a, b []byte) int {
	return bytes.Compare(a, b)
}

func (c *chromeComparer) Name() string {
	return "idb_cmp1"
}

// don't think we really use these two for query-only, hope that's right
func (c *chromeComparer) Separator(dst, a, b []byte) []byte {
	i, n := 0, len(a)
	if n > len(b) {
		n = len(b)
	}
	for ; i < n && a[i] == b[i]; i++ {
	}
	if i >= n {
		// Do not shorten if one string is a prefix of the other
	} else if c := a[i]; c < 0xff && c+1 < b[i] {
		dst = append(dst, a[:i+1]...)
		dst[len(dst)-1]++
		return dst
	}
	return nil
}
func (c *chromeComparer) Successor(dst, b []byte) []byte {
	for i, c := range b {
		if c != 0xff {
			dst = append(dst, b[:i+1]...)
			dst[len(dst)-1]++
			return dst
		}
	}
	return nil
}
