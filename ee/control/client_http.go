package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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

// controlRequest is the payload sent in control server requests
type controlRequest struct {
	// TODO: This is a temporary and simple data format for phase 1
	Message string `json:"message"`
}

func NewControlHTTPClient(addr string, opts ...HTTPClientOption) (*HTTPClient, error) {
	baseURL, err := url.Parse(fmt.Sprintf("https://%s", addr))
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}
	c := &HTTPClient{
		baseURL: baseURL,
		client:  http.DefaultClient,
		addr:    addr,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

func (c *HTTPClient) Get(subsystem, cachedETag string) (etag string, data io.Reader, err error) {
	verb, path := "GET", "/api/v1/control"
	params := &controlRequest{
		Message: "ping",
	}

	response, err := c.do(verb, path, cachedETag, params)
	if err != nil {
		level.Error(c.logger).Log(
			"msg", "error making request to control server endpoint",
			"err", err,
		)
		return "", nil, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusNotFound:
		// This could indicate an inconsistency in server data, or a client logic error
		level.Error(c.logger).Log(
			"msg", "got HTTP 404 making control server request",
			"err", err,
		)
		return "", nil, err

	case http.StatusNotModified:
		// The control server sends back a 304 Not Modified status, without a body, which tells
		// the client that the cached version of the response is still good to use
		level.Debug(c.logger).Log(
			"msg", "got HTTP 304 making control server request",
			"err", err,
		)
		return "", nil, err
	}

	if response.StatusCode != http.StatusOK {
		level.Error(c.logger).Log(
			"msg", "got not-ok status code from control server",
			"response_code", response.StatusCode,
		)
		return "", nil, err
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		level.Error(c.logger).Log(
			"msg", "error reading response body from control server",
			"err", err,
		)
		return "", nil, err
	}

	reader := bytes.NewReader(body)
	return "", reader, nil
}

func (c *HTTPClient) do(verb, path, etag string, params interface{}) (*http.Response, error) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	if etag != "" {
		// If we have a cached version of this resource, include the etag so the server
		// can compare the client's etag with its current version of the resource.
		headers["If-None-Match"] = etag
	}

	return c.doWithHeaders(verb, path, params, headers)
}

func (c *HTTPClient) doWithHeaders(verb, path string, params interface{}, headers map[string]string) (*http.Response, error) {
	var bodyBytes []byte
	var err error
	if params != nil {
		bodyBytes, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshaling json: %w", err)
		}
	}

	request, err := http.NewRequest(
		verb,
		c.url(path).String(),
		bytes.NewBuffer(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("creating request object: %w", err)
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}

	return c.client.Do(request)
}

func (c *HTTPClient) url(path string) *url.URL {
	u := *c.baseURL
	u.Path = path
	return &u
}
