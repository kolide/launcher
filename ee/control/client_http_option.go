package control

import (
	"crypto/tls"
	"net/http"
)

type HTTPClientOption func(*HTTPClient)

func WithInsecureSkipVerify() HTTPClientOption {
	return func(c *HTTPClient) {
		c.client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		c.insecure = true
	}
}

func WithDisableTLS() HTTPClientOption {
	return func(c *HTTPClient) {
		c.disableTLS = true
	}
}
