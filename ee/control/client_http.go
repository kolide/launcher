package control

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// HTTPClient handles retrieving control data via HTTP
type HTTPClient struct {
	logger     log.Logger
	addr       string
	baseURL    *url.URL
	client     *http.Client
	insecure   bool
	disableTLS bool
}

func NewControlHTTPClient(logger log.Logger, addr string, client *http.Client, opts ...HTTPClientOption) (*HTTPClient, error) {
	baseURL, err := url.Parse(fmt.Sprintf("https://%s", addr))
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}
	c := &HTTPClient{
		logger:  logger,
		baseURL: baseURL,
		client:  client,
		addr:    addr,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

func (c *HTTPClient) Get(hash string) (data io.Reader, err error) {
	verb, path := "GET", fmt.Sprintf("/api/v1/control/%s", hash)

	response, err := c.do(verb, path)
	if err != nil {
		level.Error(c.logger).Log(
			"msg", "error making request to control server endpoint",
			"err", err,
		)
		return nil, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusNotFound:
		// This could indicate an inconsistency in server data, or a client logic error
		level.Error(c.logger).Log(
			"msg", "got HTTP 404 making control server request",
		)
		return nil, err

	case http.StatusNotModified:
		// The control server sends back a 304 Not Modified status, without a body, which tells
		// the client that the cached version of the response is still good to use
		level.Debug(c.logger).Log(
			"msg", "got HTTP 304 making control server request",
		)
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		level.Error(c.logger).Log(
			"msg", "got not-ok status code from control server",
			"response_code", response.StatusCode,
			"response_body", response.Body,
		)
		return nil, err
	}

	// response.Body will be closed before this function exits, get all the data now
	body, err := io.ReadAll(response.Body)
	if err != nil {
		level.Debug(c.logger).Log(
			"msg", "error reading response body from control server",
			"err", err,
		)
		return nil, err
	}

	reader := bytes.NewReader(body)
	return reader, nil
}

func (c *HTTPClient) do(verb, path string) (*http.Response, error) {
	request, err := http.NewRequest(
		verb,
		c.url(path).String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating request object: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	return c.client.Do(request)
}

func (c *HTTPClient) url(path string) *url.URL {
	u := *c.baseURL
	u.Path = path
	return &u
}
