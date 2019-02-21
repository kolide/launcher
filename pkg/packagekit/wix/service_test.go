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

	expectedXml := `<ServiceInstall Account="NT AUTHORITY\SYSTEM" ErrorControl="normal" Id="daemonSvc" Name="daemonSvc" Start="auto" Type="ownProcess" Vital="yes"></ServiceInstall>
                    <ServiceControl Name="daemonSvc" Id="daemonSvc" Remove="uninstall" Start="install" Stop="both" Wait="no"></ServiceControl>`

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
			out: `<ServiceInstall Account="NT AUTHORITY\SYSTEM" ErrorControl="normal" Id="myDaemon" Name="myDaemon" Start="auto" Type="ownProcess" Vital="yes"></ServiceInstall>
                    <ServiceControl Name="myDaemon" Id="myDaemon" Remove="uninstall" Start="install" Stop="both" Wait="no"></ServiceControl>`,
		},
		{
			in: NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first"})),
			out: `<ServiceInstall Account="NT AUTHORITY\SYSTEM" Arguments="first" ErrorControl="normal" Id="myDaemon" Name="myDaemon" Start="auto" Type="ownProcess" Vital="yes"></ServiceInstall>
                    <ServiceControl Name="myDaemon" Id="myDaemon" Remove="uninstall" Start="install" Stop="both" Wait="no"></ServiceControl>`,
		},
		{
			in: NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first with spaces"})),
			out: `<ServiceInstall Account="NT AUTHORITY\SYSTEM" Arguments="first\ with\ spaces" ErrorControl="normal" Id="myDaemon" Name="myDaemon" Start="auto" Type="ownProcess" Vital="yes"></ServiceInstall>
                    <ServiceControl Name="myDaemon" Id="myDaemon" Remove="uninstall" Start="install" Stop="both" Wait="no"></ServiceControl>`,
		},

		{
			in: NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first", "second"})),
			out: `<ServiceInstall Account="NT AUTHORITY\SYSTEM" Arguments="first second" ErrorControl="normal" Id="myDaemon" Name="myDaemon" Start="auto" Type="ownProcess" Vital="yes"></ServiceInstall>
                    <ServiceControl Name="myDaemon" Id="myDaemon" Remove="uninstall" Start="install" Stop="both" Wait="no"></ServiceControl>`,
		},

		{
			in: NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first", "second", "third has spaces"})),
			out: `<ServiceInstall Account="NT AUTHORITY\SYSTEM" Arguments="first second third\ has\ spaces" ErrorControl="normal" Id="myDaemon" Name="myDaemon" Start="auto" Type="ownProcess" Vital="yes"></ServiceInstall>
                    <ServiceControl Name="myDaemon" Id="myDaemon" Remove="uninstall" Start="install" Stop="both" Wait="no"></ServiceControl>`,
		},
	}

	for _, tt := range tests {
		var xmlString bytes.Buffer
		err := tt.in.Xml(&xmlString)
		require.NoError(t, err)
		require.Equal(t, tt.out, strings.TrimSpace(xmlString.String()))
	}

}
