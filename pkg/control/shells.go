package control

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/ptycmd"
	"github.com/kolide/launcher/pkg/webtty"
	"github.com/kolide/launcher/pkg/wsrelay"
)

type getShellsRequest struct {
	NodeKey string `json:"node_key"`
}

type getShellsResponse struct {
	Sessions    []map[string]string `json:"sessions"`
	Err         string              `json:"error,omitempty"`
	NodeInvalid bool                `json:"node_invalid,omitempty"`
}

func (c *Client) getShells(ctx context.Context) {
	nodeKey, err := osquery.NodeKeyFromDB(c.db)
	if err != nil {
		level.Debug(c.logger).Log(
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
		level.Debug(c.logger).Log(
			"msg", "error making request to get shells endpoint",
			"err", err,
		)
		return
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusNotFound:
		level.Debug(c.logger).Log(
			"msg", "got 404 making get shells request",
			"err", err,
		)
		return
	}

	if response.StatusCode != http.StatusOK {
		level.Debug(c.logger).Log(
			"msg", "got not-ok status code getting shells",
			"response_code", response.StatusCode,
		)
		return
	}

	var responseBody getShellsResponse
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		level.Debug(c.logger).Log(
			"msg", "error decoding get shells json",
			"err", err,
		)
		return
	}

	if responseBody.Err != "" {
		level.Debug(c.logger).Log(
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

		// for every shell, handle the shell in a goroutine
		for _, session := range responseBody.Sessions {
			go c.connectToShell(ctx, path, session)
		}
	}
}

func (c *Client) connectToShell(ctx context.Context, path string, session map[string]string) {
	room, ok := session["session_id"]
	if !ok {
		level.Debug(c.logger).Log(
			"msg", "session didn't contain id",
		)
		return
	}

	secret, ok := session["secret"]
	if !ok {
		level.Debug(c.logger).Log(
			"msg", "session didn't contain secret",
		)
		return
	}

	wsPath := path + "/" + room
	client, err := wsrelay.NewClient(c.addr, wsPath, c.disableTLS, c.insecure)
	if err != nil {
		level.Debug(c.logger).Log(
			"msg", "error creating client",
			"err", err,
		)
		return
	}
	defer client.Close()

	pty, err := ptycmd.NewCmd("/bin/bash", []string{"--login"})
	if err != nil {
		level.Debug(c.logger).Log(
			"msg", "error creating PTY command",
			"err", err,
		)
		return
	}

	TTY, err := webtty.New(
		client,
		pty,
		secret,
		webtty.WithPermitWrite(),
		webtty.WithLogger(c.logger),
		webtty.WithKeepAliveDeadline(),
	)
	if err != nil {
		level.Debug(c.logger).Log(
			"msg", "error creating TTY",
			"err", err,
		)
	}
	if err := TTY.Run(ctx); err != nil {
		level.Debug(c.logger).Log(
			"msg", "error running TTY",
			"err", err,
		)
		return
	}
}
