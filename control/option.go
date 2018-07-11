package control

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
)

type Option func(*Client)

func WithLogger(logger log.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

func WithInsecureSkipVerify() Option {
	return func(c *Client) {
		c.client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		c.insecure = true
	}
}

func WithGetShellsInterval(i time.Duration) Option {
	return func(c *Client) {
		c.getShellsInterval = i
	}
}
