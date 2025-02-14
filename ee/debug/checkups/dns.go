package checkups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
)

type (
	HostResolver interface {
		LookupHost(ctx context.Context, host string) ([]string, error)
	}
	dnsCheckup struct {
		k        types.Knapsack
		status   Status
		summary  string
		data     map[string]any
		resolver HostResolver
	}
)

func (dc *dnsCheckup) Data() any             { return dc.data }
func (dc *dnsCheckup) ExtraFileName() string { return "" }
func (dc *dnsCheckup) Name() string          { return "DNS Resolution" }
func (dc *dnsCheckup) Status() Status        { return dc.status }
func (dc *dnsCheckup) Summary() string       { return dc.summary }

func (dc *dnsCheckup) Run(ctx context.Context, extraFH io.Writer) error {
	if dc.resolver == nil {
		dc.resolver = &net.Resolver{}
	}

	hosts := []string{
		dc.k.KolideServerURL(),
		dc.k.ControlServerURL(),
		dc.k.TufServerURL(),
		"google.com",
		"apple.com",
	}

	dc.data = make(map[string]any)
	attemptedCount, successCount := 0, 0

	for _, host := range hosts {
		if len(strings.TrimSpace(host)) == 0 {
			continue
		}

		hostUrl, err := parseUrl(dc.k, host)
		if err != nil {
			dc.data[host] = fmt.Sprintf("PARSE ERROR: %s", err.Error())
			continue
		}

		ips, err := dc.resolveHost(ctx, hostUrl.Hostname())
		// keep attemptedCount as a separate variable to avoid indicating failures where we didn't even try
		attemptedCount++

		if err != nil {
			dc.data[hostUrl.Hostname()] = fmt.Sprintf("RESOLVE ERROR: %s", err.Error())
			continue
		}

		dc.data[hostUrl.Hostname()] = ips
		successCount++
	}

	if successCount == attemptedCount {
		dc.status = Passing
	} else if successCount > 0 {
		dc.status = Warning
	} else {
		dc.status = Failing
	}

	dc.data["lookup_attempts"] = attemptedCount
	dc.data["lookup_successes"] = successCount

	dc.summary = fmt.Sprintf("successfully resolved %d/%d hosts", successCount, attemptedCount)

	return nil
}

func (dc *dnsCheckup) resolveHost(ctx context.Context, host string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	ips, err := dc.resolver.LookupHost(ctx, host)

	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", errors.New("host was valid but did not resolve")
	}

	return strings.Join(ips, ","), nil
}
