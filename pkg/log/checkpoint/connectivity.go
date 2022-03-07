package checkpoint

import (
	"fmt"
	"net"
)

type dialer interface {
	Dial(network, address string) (net.Conn, error)
}

func testConnections(dialer dialer, hosts ...string) []string {
	results := []string{}

	for _, host := range hosts {
		if err := testConnection(dialer, host); err != nil {
			results = append(results, fmt.Sprintf("%s: %s", host, err.Error()))
		} else {
			results = append(results, fmt.Sprintf("%s: success", host))
		}
	}

	return results
}

func testConnection(dialer dialer, host string) error {
	hostPort := net.JoinHostPort(host, "443")
	conn, err := dialer.Dial("tcp", hostPort)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}
