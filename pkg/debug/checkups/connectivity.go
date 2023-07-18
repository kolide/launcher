package checkups

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/pkg/errors"
)

const requestTimeout = time.Second * 5

type Connectivity struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]string
}

func (c *Connectivity) Name() string {
	return "Check communication with Kolide"
}

func (c *Connectivity) Run(ctx context.Context, extraFH io.Writer) error {
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

func (c *Connectivity) ExtraFileName() string {
	return "responses.txt"
}

func (c *Connectivity) Status() Status {
	return c.status
}

func (c *Connectivity) Summary() string {
	return c.summary
}

func (c *Connectivity) Data() any {
	return c.data
}

func checkKolideServer(k types.Knapsack, client *http.Client, fh io.Writer, server string) ([]byte, error) {
	parsedUrl, err := parseUrl(k, fmt.Sprintf("%s/version", server))
	if err != nil {
		return nil, fmt.Errorf("parsing url(%s): %w", server, err)
	}

	response, err := client.Get(parsedUrl.String())
	if err != nil {
		return nil, fmt.Errorf("fetching url: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("expected status 200, got %d", response.StatusCode)
	}

	bytes, err := io.ReadAll(io.LimitReader(response.Body, 256))
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	return bytes, nil
}

func parseUrl(k types.Knapsack, addr string) (*url.URL, error) {
	if !strings.HasPrefix(addr, "http") {
		scheme := "https"
		if k.InsecureTransportTLS() {
			scheme = "http"
		}
		addr = fmt.Sprintf("%s://%s", scheme, addr)
	}

	u, err := url.Parse(addr)

	if err != nil {
		return nil, err
	}

	if u.Port() == "" {
		port := "443"
		if k.InsecureTransportTLS() {
			port = "80"
		}
		u.Host = net.JoinHostPort(u.Host, port)
	}

	return u, nil
}
