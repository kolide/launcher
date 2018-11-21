// +build !dockertest

package osquery

func serialForRow(row map[string]string) string {
	return row["hardware_serial"]
}
