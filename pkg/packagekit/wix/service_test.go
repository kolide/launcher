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

	expectedXml := `<ServiceInstall Account="NT AUTHORITY\SYSTEM" ErrorControl="normal" Id="DaemonSvc" Name="DaemonSvc" Start="auto" Type="ownProcess" Vital="yes">
                        <ServiceConfig xmlns="http://schemas.microsoft.com/wix/UtilExtension" FirstFailureActionType="restart" SecondFailureActionType="restart" ThirdFailureActionType="restart" RestartServiceDelayInSeconds="5" ResetPeriodInDays="1"></ServiceConfig>
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
			in:  NewService("snake-case.exe"),
			out: []string{`Id="SnakeCaseSvc"`, `Name="SnakeCaseSvc"`},
		},
		{
			in:  NewService("snake-case.exe", ServiceName("Another-Case-Of-snakes")),
			out: []string{`Id="AnotherCaseOfSnakes"`, `Name="AnotherCaseOfSnakes"`},
		},
		{
			in:  NewService("daemon.exe"),
			out: []string{`Id="DaemonSvc"`, `Name="DaemonSvc"`},
		},
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon")),
			out: []string{`Id="MyDaemon"`, `Name="MyDaemon"`},
		},
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon"), ServiceArgs([]string{"first"})),
			out: []string{`Id="MyDaemon"`, `Name="MyDaemon"`, `Arguments="first"`},
		},
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon.svc"), ServiceArgs([]string{"first with spaces"})),
			out: []string{`Id="MyDaemonSvc"`, `Name="MyDaemonSvc"`, `Arguments="&#34;first with spaces&#34;"`},
		},

		{
			in:  NewService("daemon.exe", ServiceName("myDaemon svc"), ServiceArgs([]string{"first", "second"})),
			out: []string{`Id="MyDaemonSvc"`, `Name="MyDaemonSvc"`, `Arguments="first second"`},
		},
		{
			in:  NewService("daemon.exe", ServiceName("myDaemon_svc"), ServiceArgs([]string{"first", "second", "third has spaces"})),
			out: []string{`Id="MyDaemonSvc"`, `Name="MyDaemonSvc"`, `Arguments="first second &#34;third has spaces&#34;"`},
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
