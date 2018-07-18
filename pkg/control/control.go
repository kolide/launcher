package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
)

type Client struct {
	addr              string
	baseURL           *url.URL
	cancel            context.CancelFunc
	client            *http.Client
	db                *bolt.DB
	getShellsInterval time.Duration
	insecure          bool
	disableTLS        bool
	logger            log.Logger
}

func NewControlClient(db *bolt.DB, addr string, opts ...Option) (*Client, error) {
	baseURL, err := url.Parse("https://" + addr)
	if err != nil {
		return nil, errors.Wrap(err, "parsing URL")
	}
	c := &Client{
		logger:            log.NewNopLogger(),
		baseURL:           baseURL,
		client:            http.DefaultClient,
		db:                db,
		addr:              addr,
		getShellsInterval: 5 * time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.disableTLS {
		c.baseURL.Scheme = "http"
	}

	return c, nil
}

func (c *Client) Start(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)
	getShellsTicker := time.NewTicker(c.getShellsInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-getShellsTicker.C:
			c.getShells(ctx)
		}
	}
}

func (c *Client) do(verb, path string, params interface{}) (*http.Response, error) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	return c.doWithHeaders(verb, path, params, headers)
}

func (c *Client) doWithHeaders(verb, path string, params interface{}, headers map[string]string) (*http.Response, error) {
	var bodyBytes []byte
	var err error
	if params != nil {
		bodyBytes, err = json.Marshal(params)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling json")
		}
	}

	request, err := http.NewRequest(
		verb,
		c.url(path).String(),
		bytes.NewBuffer(bodyBytes),
	)
	if err != nil {
		return nil, errors.Wrap(err, "creating request object")
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}

	return c.client.Do(request)
}

func (c *Client) url(path string) *url.URL {
	u := *c.baseURL
	u.Path = path
	return &u
}

// Stop stops the client
func (c *Client) Stop() {
	c.cancel()
}
