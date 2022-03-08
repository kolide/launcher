package checkpoint

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// ipLookeruper is an interface to allow mocking of ip look ups
type ipLookuper interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

func lookupHostsIpv4s(ipLookuper ipLookuper, hosts ...string) []string {
	results := []string{}

	for _, host := range hosts {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s: ", host))

		ips, err := lookupIpv4(ipLookuper, host)

		if err != nil {
			sb.WriteString(err.Error())
		} else {
			sb.WriteString(strings.Join(ips, ", "))
		}

		results = append(results, sb.String())
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
