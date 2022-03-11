package checkpoint

import (
	"net"
	"net/url"
)

type dialer interface {
	Dial(network, address string) (net.Conn, error)
}

func testConnections(dialer dialer, urls ...*url.URL) map[string]string {
	results := make(map[string]string)

	for _, url := range urls {
		if err := testConnection(dialer, url); err != nil {
			results[url.Host] = err.Error()
		} else {
			results[url.Host] = "successful tcp connection"
		}
	}

	return results
}

func testConnection(dialer dialer, url *url.URL) error {
	conn, err := dialer.Dial("tcp", url.Host)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}
