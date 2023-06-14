package exporter

import (
	"context"
	"fmt"
)

// Implements google.golang.org/grpc/credentials.PerRPCCredentials interface
type clientAuthenticator struct {
	token string
}

func newClientAuthenticator(token string) *clientAuthenticator {
	return &clientAuthenticator{
		token: token,
	}
}

// setToken updates the token used as the bearer auth token -- this is used
// to swap the bearer auth token in-place before it expires.
func (c *clientAuthenticator) setToken(token string) {
	c.token = token
}

// GetRequestMetadata adds the necessary authentication header to the request.
func (c *clientAuthenticator) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", c.token),
	}, nil
}

// RequireTransportSecurity indicates whether the credentials requires
// transport security.
func (c *clientAuthenticator) RequireTransportSecurity() bool {
	return false
}
