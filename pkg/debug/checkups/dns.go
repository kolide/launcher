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

type dnsCheckup struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]any
}

func (nc *dnsCheckup) Data() map[string]any  { return nc.data }
func (nc *dnsCheckup) ExtraFileName() string { return "" }
func (nc *dnsCheckup) Name() string          { return "DNS Resolution" }
func (nc *dnsCheckup) Status() Status        { return nc.status }
func (nc *dnsCheckup) Summary() string       { return nc.summary }

func (dc *dnsCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	hosts := []string{
		dc.k.KolideServerURL(),
		dc.k.ControlServerURL(),
		dc.k.LogIngestServerURL(),
		dc.k.TufServerURL(),
		"https://google.com",
		"https://apple.com",
	}

	dc.data = make(map[string]any)
	attemptedCount, successCount := 0, 0
	resolver := &net.Resolver{}

	for _, host := range hosts {
		if len(strings.TrimSpace(host)) == 0 {
			continue
		}

		parsedUrl, err := url.Parse(host)
		if err != nil {
			dc.data[host] = fmt.Sprintf("unable to parse url from host: %s", err.Error())
			continue
		}

		ips, err := resolveHost(resolver, parsedUrl.Host)
		// keep attemptedCount as a separate variable to avoid indicating failures where we didn't even try
		attemptedCount++

		if err != nil {
			dc.data[parsedUrl.Host] = fmt.Sprintf("ERROR: %s", err.Error())
			continue
		}

		dc.data[parsedUrl.Host] = ips
		successCount++
	}

	if successCount == len(hosts) {
		dc.status = Passing
	} else if successCount > 0 {
		dc.status = Warning
	} else {
		dc.status = Failing
	}

	dc.summary = fmt.Sprintf("successfully resolved %d/%d hosts", successCount, attemptedCount)

	return nil
}

func resolveHost(resolver *net.Resolver, host string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ips, err := resolver.LookupHost(ctx, host)

	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("host was valid but did not resolve")
	}

	return strings.Join(ips, ", "), nil
}
