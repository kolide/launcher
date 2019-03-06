// +build fakeserial

package osquery

import "github.com/kolide/kit/ulid"

var fakeSerialNumber = ulid.New()

func serialForRow(row map[string]string) string {
	return fakeSerialNumber
}
