package dataflatten

import "strings"

// Row is the record type we return.
type Row struct {
	Path  []string
	Value string
}

func (r Row) StringPath(sep string) string {
	return strings.Join(r.Path, sep)
}

func (r Row) ParentKey(sep string) (string, string) {
	switch len(r.Path) {
	case 0:
		return "", ""
	case 1:
		return "", r.Path[0]
	}

	parent := strings.Join(r.Path[:len(r.Path)-1], sep)
	key := r.Path[len(r.Path)-1]

	return parent, key
}
