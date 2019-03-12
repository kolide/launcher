package wix

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	t.Parallel()
	t.Skip()

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

	expectedXml := `<ServiceInstall Account="NT AUTHORITY\SYSTEM" ErrorControl="normal" Id="DaemonSvc" Name="DaemonSvc" Start="auto" Type="ownProcess" Vital="yes">
                        <util:ServiceConfig FirstFailureActionType="restart" SecondFailureActionType="restart" ThirdFailureActionType="restart" RestartServiceDelayInSeconds="5" ResetPeriodInDays="1"></util:ServiceConfig>
                        <ServiceConfig OnInstall="yes" OnReinstall="yes" FailureActionsWhen="failedToStopOrReturnedError"></ServiceConfig>
                    </ServiceInstall>
                    <ServiceControl Name="DaemonSvc" Id="DaemonSvc" Remove="uninstall" Start="install" Stop="both" Wait="no"></ServiceControl>`

	var xmlString bytes.Buffer

	err = service.Xml(&xmlString)
	require.NoError(t, err)
	require.Equal(t, expectedXml, strings.TrimSpace(xmlString.String()))
}

func TestServiceOptions(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  *Service
		out []string
	}{
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon")),
			out: []string{`Id="myDaemon"`, `Name="myDaemon"`},
		},
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first"})),
			out: []string{`Id="myDaemon"`, `Name="myDaemon"`, `Arguments="first"`},
		},
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first with spaces"})),
			out: []string{`Id="myDaemon"`, `Name="myDaemon"`, `Arguments="&#34;first with spaces&#34;"`},
		},

		{
			in:  NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first", "second"})),
			out: []string{`Id="myDaemon"`, `Name="myDaemon"`, `Arguments="first second"`},
		},
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first", "second", "third has spaces"})),
			out: []string{`Id="myDaemon"`, `Name="myDaemon"`, `Arguments="first second &#34;third has spaces&#34;"`},
		},
	}

	for _, tt := range tests {
		var xmlString bytes.Buffer
		err := tt.in.Xml(&xmlString)
		require.NoError(t, err)
		for _, outStr := range tt.out {
			require.Contains(t, strings.TrimSpace(xmlString.String()), outStr)
		}
	}

}
