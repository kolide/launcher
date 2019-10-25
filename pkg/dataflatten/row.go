package dataflatten

import "strings"

type Row struct {
	Path  []string
	Value string
}

func (r Row) StringPath() string {
	return strings.Join(r.Path, defaultPathSeperator)
}

func (r Row) ParentKey() (string, string) {
	switch len(r.Path) {
	case 0:
		return "", ""
	case 1:
		return "", r.Path[0]
	}

	parent := strings.Join(r.Path[:len(r.Path)-1], defaultPathSeperator)
	key := r.Path[len(r.Path)-1]

	return parent, key
}
