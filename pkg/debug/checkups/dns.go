package checkups

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/kolide/launcher/pkg/agent/types"
)

type dnsCheck struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]string
}

// ipLookeruper is an interface to allow mocking of ip look ups
type ipLookuper interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

func (dc *dnsCheck) Name() string          { return "Verify DNS Resolution" }
func (dc *dnsCheck) Data() any             { return dc.data }
func (dc *dnsCheck) ExtraFileName() string { return "resolutions.txt" }
func (dc *dnsCheck) Status() Status        { return dc.status }
func (dc *dnsCheck) Summary() string       { return dc.summary }

func (dc *dnsCheck) Run(ctx context.Context, extraFH io.Writer) error {
	hosts := []string{
		dc.k.KolideServerURL(),
		dc.k.ControlServerURL(),
		dc.k.TraceIngestServerURL(),
		dc.k.LogIngestServerURL(),
	}

	dc.data = make(map[string]string)

	successCount := 0
	ipLookerUpperer := &net.Resolver{}
	fmt.Printf("HOSTS %v", hosts)

	for _, host := range hosts {
		parsedUrl, err := url.Parse(host)
		if err != nil {
			dc.data[host] = fmt.Sprintf("unable to parse url from host: %w", err)
			continue
		}

		ips, err := lookupIpv4(ipLookerUpperer, parsedUrl)

		if err != nil {
			dc.data[parsedUrl.Hostname()] = err.Error()
			continue
		}

		dc.data[parsedUrl.Hostname()] = ips
		successCount++
	}

	if successCount == len(hosts) {
		dc.status = Passing
	} else if successCount > 0 {
		dc.status = Warning
	} else {
		dc.status = Failing
	}

	dc.summary = fmt.Sprintf("successfully resolved %d/%d hosts", successCount, len(hosts))

	return nil
}

func lookupIpv4(ipLookuper ipLookuper, url *url.URL) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ips, err := ipLookuper.LookupIP(ctx, "ip", url.Hostname())

	if err != nil {
		return "", err
	}

	ipv4s := []string{}
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipv4s = append(ipv4s, ipv4.String())
		}
	}

	return strings.Join(ipv4s, ","), nil
}
