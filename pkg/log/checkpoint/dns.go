package checkpoint

import (
	"context"
	"net"
	"net/url"
)

// ipLookeruper is an interface to allow mocking of ip look ups
type ipLookuper interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

func lookupHostsIpv4s(ipLookuper ipLookuper, urls ...*url.URL) map[string]interface{} {
	results := make(map[string]interface{})

	for _, url := range urls {
		ips, err := lookupIpv4(ipLookuper, url)

		if err != nil {
			results[url.Hostname()] = err.Error()
		} else {
			results[url.Hostname()] = ips
		}
	}

	return results
}

func lookupIpv4(ipLookuper ipLookuper, url *url.URL) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ips, err := ipLookuper.LookupIP(ctx, "ip", url.Hostname())

	if err != nil {
		return nil, err
	}

	ipv4s := []string{}
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipv4s = append(ipv4s, ipv4.String())
		}
	}

	return ipv4s, nil
}
