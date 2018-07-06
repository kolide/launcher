package control

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/broker/pkg/ptycmd"
	"github.com/kolide/broker/pkg/webtty"
	"github.com/kolide/broker/pkg/wsrelay"
	"github.com/kolide/launcher/osquery"
	"github.com/pkg/errors"
)

type Client struct {
	logger            log.Logger
	baseURL           *url.URL
	client            *http.Client
	db                *bolt.DB
	addr              string
	getShellsInterval time.Duration
	cancel            context.CancelFunc
}

func NewControlClient(logger log.Logger, db *bolt.DB, addr string, insecureSkipVerify bool) (*Client, error) {
	baseURL, err := url.Parse(addr)
	if err != nil {
		return nil, errors.Wrap(err, "parsing URL")
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
		},
	}
	return &Client{
		logger:            logger,
		baseURL:           baseURL,
		client:            client,
		db:                db,
		addr:              addr,
		getShellsInterval: 5 * time.Second,
	}, nil
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

func (c *Client) getShells(ctx context.Context) {
	nodeKey, err := osquery.NodeKeyFromDB(c.db)
	if err != nil {
		level.Info(c.logger).Log(
			"msg", "error getting node key from db to request shells",
			"err", err,
		)
		return
	}

	verb, path := "POST", "/api/v1/shells"
	params := &getShellsRequest{
		NodeKey: nodeKey,
	}
	response, err := c.do(verb, path, params)
	if err != nil {
		level.Info(c.logger).Log(
			"msg", "error making request to get shells endpoint",
			"err", err,
		)
		return
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusNotFound:
		level.Info(c.logger).Log(
			"msg", "got 404 making get shells request",
			"err", err,
		)
		return
	}

	if response.StatusCode != http.StatusOK {
		level.Info(c.logger).Log(
			"msg", "got not-ok status code getting shells",
			"response_code", response.StatusCode,
		)
		return
	}

	var responseBody getShellsResponse
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		level.Info(c.logger).Log(
			"msg", "error decoding get shells json",
			"err", err,
		)
		return
	}

	if responseBody.Err != nil {
		level.Info(c.logger).Log(
			"msg", "response body contained error",
			"err", responseBody.Err,
		)
		return
	}

	if len(responseBody.Sessions) > 0 {
		level.Debug(c.logger).Log(
			"msg", "found shell session requests",
			"count", len(responseBody.Sessions),
		)

		for _, session := range responseBody.Sessions {
			room, ok := session["session_id"]
			if !ok {
				level.Info(c.logger).Log(
					"msg", "session didn't contain id",
				)
				return
			}

			secret, ok := session["secret"]
			if !ok {
				level.Info(c.logger).Log(
					"msg", "session didn't contain secret",
				)
				return
			}

			// TODO(logan): modify the wsrelay code and use the secret
			_ = secret

			client, err := wsrelay.NewClient(c.addr, room)
			if err != nil {
				level.Info(c.logger).Log(
					"msg", "error creating client",
					"err", err,
				)
				return
			}
			defer client.Close()

			pty, err := ptycmd.NewCmd("/bin/bash", []string{})
			if err != nil {
				level.Info(c.logger).Log(
					"msg", "error creating PTY command",
					"err", err,
				)
				return
			}

			TTY, err := webtty.New(client, pty, webtty.WithPermitWrite())
			if err := TTY.Run(ctx); err != nil {
				level.Info(c.logger).Log(
					"msg", "error creating web TTY",
					"err", err,
				)
				return
			}
		}
	}
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

func (c *Client) do(verb, path string, params interface{}) (*http.Response, error) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	return c.doWithHeaders(verb, path, params, headers)
}

func (c *Client) url(path string) *url.URL {
	u := *c.baseURL
	u.Path = path
	return &u
}

func (c *Client) Stop() {
	c.cancel()
}

type getShellsRequest struct {
	NodeKey string `json:"node_key"`
}

type getShellsResponse struct {
	Sessions    []map[string]string `json:"sessions"`
	Err         error               `json:"error,omitempty"`
	NodeInvalid bool                `json:"node_invalid,omitempty"`
}
