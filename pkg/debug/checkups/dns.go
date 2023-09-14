package checkups

import (
	"context"
	"net"
	"net/url"

	"github.com/kolide/launcher/pkg/agent/types"
)

type dnsCheck struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]string
}

func (dc *dnsCheck) Name() string {
	return "Check DNS Resolution Health"
}

func (dc *dnsCheck) Run(ctx context.Context, extraFH io.Writer) error {
	//if !c.k.KolideHosted() {
	//	c.status = Unknown
	//	c.summary = "not kolide hosted"
	//	return nil
	//}

	httpClient := &http.Client{Timeout: requestTimeout}

	hosts := map[string]string{
		"device":  c.k.KolideServerURL(),
		"control": c.k.ControlServerURL(),
		"trace":   c.k.TraceIngestServerURL(),
		"log":     c.k.LogIngestServerURL(),
	}

	c.data = make(map[string]string, len(hosts))

	failingHosts := make([]string, 0)
	for n, v := range hosts {
		fmt.Fprintf(extraFH, "Response from %s / %s:\n", n, v)
		if v == "" {
			fmt.Fprintf(extraFH, "%s\n", "not in knapsack")
			c.data[n] = "not in knapsack"
			continue
		}

		body, err := checkKolideServer(c.k, httpClient, extraFH, v)
		if err != nil {
			fmt.Fprintf(extraFH, "error: %s\n", err)
			c.data[n] = err.Error()
			failingHosts = append(failingHosts, fmt.Sprintf("%s(%s)", n, v))
			continue
		}

		fmt.Fprintf(extraFH, "%s\n", string(body))
		c.data[n] = string(body)
	}

	if len(failingHosts) > 0 {
		c.status = Failing
		c.summary = fmt.Sprintf("Trouble connecting to: %s", strings.Join(failingHosts, ", "))
		return nil
	}
	c.status = Passing
	c.summary = "successfully connected to device and control server"
	return nil
}

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