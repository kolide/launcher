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
)

const requestTimeout = time.Second * 5

type Connectivity struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (c *Connectivity) Name() string {
	return "Check communication with Kolide"
}

func (c *Connectivity) Run(ctx context.Context, fullFH io.Writer) error {
	//if !c.k.KolideHosted() {
	//	c.status = Unknown
	//	c.summary = "not kolide hosted"
	//	return nil
	//}

	httpClient := &http.Client{Timeout: requestTimeout}

	hosts := map[string]string{
		"device server":  c.k.KolideServerURL(),
		"control server": c.k.ControlServerURL(),
	}

	for n, v := range hosts {
		fmt.Fprintf(fullFH, "Response from %s / %s:\n", n, v)
		if err := checkKolideServer(c.k, httpClient, fullFH, v); err != nil {
			fmt.Fprintf(fullFH, "\n")
			return fmt.Errorf("%s(%s): %w", n, v, err)
		}
		fmt.Fprintf(fullFH, "\n")
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
	return nil
}

func checkKolideServer(k types.Knapsack, client *http.Client, fh io.Writer, server string) error {
	parsedUrl, err := parseUrl(k, fmt.Sprintf("%s/version", server))
	if err != nil {
		return fmt.Errorf("parsing url(%s): %w", server, err)
	}

	response, err := client.Get(parsedUrl.String())
	if err != nil {
		return fmt.Errorf("fetching url: %w", err)
	}
	defer response.Body.Close()

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}

	fh.Write(bytes)

	return nil
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
