package authedclient

import (
	"fmt"
	"net/http"
	"time"
)

type transport struct {
	http.Transport
	authToken string
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t.authToken))
	return t.Transport.RoundTrip(req)
}

type authedClient struct {
	http.Client
}

// New returns a new authed client that will add the provided auth token as an
// Authorization header to each request.
func New(authToken string, timeout time.Duration) authedClient {
	transport := &transport{
		authToken: authToken,
	}

	client := authedClient{
		Client: http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}

	return client
}
