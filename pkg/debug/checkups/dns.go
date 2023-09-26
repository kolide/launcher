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
	data    map[string]string
}

func (nc *dnsCheckup) Data() any             { return nc.data }
func (nc *dnsCheckup) ExtraFileName() string { return "" }
func (nc *dnsCheckup) Name() string          { return "DNS Resolution" }
func (nc *dnsCheckup) Status() Status        { return nc.status }
func (nc *dnsCheckup) Summary() string       { return nc.summary }

func (nc *dnsCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	hosts := onlyValidHosts( // remove any empty/missing hosts from lists
		[]string{
			nc.k.KolideServerURL(),
			nc.k.ControlServerURL(),
			nc.k.TraceIngestServerURL(),
			nc.k.LogIngestServerURL(),
			"google.com",
			"apple.com",
		},
	)

	nc.data = make(map[string]string)
	successCount := 0
	resolver := &net.Resolver{}

	for _, host := range hosts {
		_, err := url.Parse(host)
		if err != nil {
			nc.data[host] = fmt.Sprintf("unable to parse url from host: %w", err)
			continue
		}

		ips, err := resolveHost(resolver, host)

		if err != nil {
			nc.data[host] = err.Error()
			continue
		}

		nc.data[host] = ips
		successCount++
	}

	if successCount == len(hosts) {
		nc.status = Passing
	} else if successCount > 0 {
		nc.status = Warning
	} else {
		nc.status = Failing
	}

	nc.summary = fmt.Sprintf("successfully resolved %d/%d hosts", successCount, len(hosts))

	return nil
}

func resolveHost(resolver *net.Resolver, host string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ips, err := resolver.LookupHost(ctx, host)

	if err != nil {
		return "", err
	}

	return strings.Join(ips, ", "), nil
}

func onlyValidHosts(hosts []string) []string {
	filtered := make([]string, 0)
	for _, host := range hosts {
		if len(strings.TrimSpace(host)) == 0 {
			continue
		}

		filtered = append(filtered, host)
	}

	return filtered
}
