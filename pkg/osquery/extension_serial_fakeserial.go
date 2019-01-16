// +build fakeserial

package osquery

import "github.com/kolide/kit/ulid"

func serialForRow(row map[string]string) string {
	return ulid.New()
}
