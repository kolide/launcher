package checkups

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/pkg/errors"
)

const requestTimeout = time.Second * 5

type Connectivity struct {
	k       types.Knapsack
	status  Status
	summary string
	data    map[string]any
}

func (c *Connectivity) Name() string {
	return "Check communication with Kolide"
}

func (c *Connectivity) Run(ctx context.Context, extraFH io.Writer) error {
	if !c.k.KolideHosted() {
		c.status = Unknown
		c.summary = "not kolide hosted"
		return nil
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	hosts := map[string]string{
		"device":  c.k.KolideServerURL(),
		"control": c.k.ControlServerURL(),
		"trace":   c.k.TraceIngestServerURL(),
		"log":     c.k.LogIngestServerURL(),
	}

	c.data = make(map[string]any)

	failingHosts := make([]string, 0)
	attemptedHosts := make(map[string]string)
	for n, v := range hosts {
		fmt.Fprintf(extraFH, "Response from %s / %s:\n", n, v)
		if v == "" {
			fmt.Fprintf(extraFH, "%s\n", "not in knapsack")
			c.data[n] = "not in knapsack"
			continue
		}

		parsedUrl, err := parseUrl(c.k, v)
		if err != nil {
			fmt.Fprintf(extraFH, "error: %s\n", err)
			c.data[n] = err.Error()
			failingHosts = append(failingHosts, fmt.Sprintf("%s(%s)", n, v))
			continue
		}

		versionEndpoint := fmt.Sprintf("%s://%s/version", parsedUrl.Scheme, parsedUrl.Host)

		// prevent duplicate checks if any of our configured endpoints are
		// shared (e.g. trace and log)
		if prev, ok := attemptedHosts[versionEndpoint]; ok {
			c.data[n] = prev
			continue
		}

		body, err := checkKolideServer(ctx, httpClient, versionEndpoint)
		if err != nil {
			fmt.Fprintf(extraFH, "error: %s\n", err)
			c.data[n] = err.Error()
			failingHosts = append(failingHosts, fmt.Sprintf("%s(%s)", n, v))
			continue
		}

		attemptedHosts[versionEndpoint] = string(body)
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

func checkKolideServer(ctx context.Context, client *http.Client, server string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	response, err := client.Do(req)
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
