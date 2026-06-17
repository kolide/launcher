package service

import (
	"testing"

	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/stretchr/testify/assert"
)

func TestMakeTLSConfig_ServerNameStripsPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		kolideServerURL    string
		expectedServerName string
	}{
		{
			name:               "dns hostname with port",
			kolideServerURL:    "localhost:3443",
			expectedServerName: "localhost",
		},
		{
			name:               "dns hostname without port",
			kolideServerURL:    "k2device.kolide.com",
			expectedServerName: "k2device.kolide.com",
		},
		{
			name:               "ipv4 with port",
			kolideServerURL:    "127.0.0.1:3443",
			expectedServerName: "127.0.0.1",
		},
		{
			name:               "ipv6 with port",
			kolideServerURL:    "[::1]:3443",
			expectedServerName: "::1",
		},
		{
			name:               "ipv6 without port",
			kolideServerURL:    "::1",
			expectedServerName: "::1",
		},
		{
			// Unbracketed IPv6 with a port is genuinely ambiguous (the last colon
			// could be part of the address or a port separator), so SplitHostPort
			// errors and we leave it untouched -- ServerName keeps the full string
			// and verification will fail. This matches Go convention and is
			// consistent with the bare-IPv6 caveat noted at
			// cmd/launcher/launcher.go:129. Documented here rather than "fixed".
			name:               "unbracketed ipv6 with port is left unstripped",
			kolideServerURL:    "2001:db8::1:443",
			expectedServerName: "2001:db8::1:443",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			k := typesmocks.NewKnapsack(t)
			k.On("KolideServerURL").Return(tt.kolideServerURL)
			k.On("InsecureTLS").Return(false)
			k.On("CertPins").Return([][]byte{})

			conf := makeTLSConfig(k, nil)

			assert.Equal(t, tt.expectedServerName, conf.ServerName)
		})
	}
}
