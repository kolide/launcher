package wix

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	t.Parallel()

	service := NewService("daemon.exe")

	expectFalse, err := service.Match("nomatch")
	require.NoError(t, err)
	require.False(t, expectFalse)

	expectTrue, err := service.Match("daemon.exe")
	require.NoError(t, err)
	require.True(t, expectTrue)

	// Should error. count now exceeds expectedCount.
	expectTrue2, err := service.Match("daemon.exe")
	require.Error(t, err)
	require.True(t, expectTrue2)

	expectedXml := `<ServiceInstall ErrorControl="normal" Id="ServiceInstall" Name="daemonSvc" Start="auto" Type="ownProcess"></ServiceInstall>
                    <ServiceControl Name="daemonSvc" Id="ServiceControl"></ServiceControl>`

	var xmlString bytes.Buffer

	err = service.Xml(&xmlString)
	require.NoError(t, err)
	require.Equal(t, expectedXml, strings.TrimSpace(xmlString.String()))

}

func TestServiceOptions(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  *Service
		out string
	}{
		{
			in: NewService("daemon.exe", ServiceName("myDaemon")),
			out: `<ServiceInstall ErrorControl="normal" Id="ServiceInstall" Name="myDaemon" Start="auto" Type="ownProcess"></ServiceInstall>
                    <ServiceControl Name="myDaemon" Id="ServiceControl"></ServiceControl>`,
		},
	}

	for _, tt := range tests {
		var xmlString bytes.Buffer
		err := tt.in.Xml(&xmlString)
		require.NoError(t, err)
		require.Equal(t, tt.out, strings.TrimSpace(xmlString.String()))
	}

}
