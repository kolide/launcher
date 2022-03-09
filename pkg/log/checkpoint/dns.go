package checkpoint

import (
	"context"
	"net"
)

// ipLookeruper is an interface to allow mocking of ip look ups
type ipLookuper interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

func lookupHostsIpv4s(ipLookuper ipLookuper, hosts ...string) map[string]interface{} {
	results := make(map[string]interface{})

	for _, host := range hosts {
		ips, err := lookupIpv4(ipLookuper, host)

		if err != nil {
			results[host] = err.Error()
		} else {
			results[host] = ips
		}
	}

	return results
}

func lookupIpv4(ipLookuper ipLookuper, host string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ips, err := ipLookuper.LookupIP(ctx, "ip", host)

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
