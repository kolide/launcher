package checkpoint

import (
	"net"
)

type dialer interface {
	Dial(network, address string) (net.Conn, error)
}

func testConnections(dialer dialer, hosts ...string) map[string]interface{} {
	results := make(map[string]interface{})

	for _, host := range hosts {
		if err := testConnection(dialer, host); err != nil {
			results[host] = err.Error()
		} else {
			results[host] = "successful tcp connection over 443"
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
