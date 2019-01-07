package packagekit

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderSystemdEmpty(t *testing.T) {
	t.Parallel()

	expectedOutputStrings := []string{
		`[Unit]`,
	}

	var output bytes.Buffer
	err := RenderSystemd(context.TODO(), &output, emptyInitOptions())
	require.NoError(t, err)
	require.True(t, len(output.String()) > 100)

	for _, s := range expectedOutputStrings {
		require.Contains(t, output.String(), s)
	}

}

func TestRenderSystemdComplex(t *testing.T) {
	t.Parallel()

	expectedOutputStrings := []string{
		`[Unit]`,
	}

	var output bytes.Buffer
	err := RenderSystemd(context.TODO(), &output, complexInitOptions())
	require.NoError(t, err)
	require.True(t, len(output.String()) > 100)

	for _, s := range expectedOutputStrings {
		require.Contains(t, output.String(), s)
	}

	require.Equal(t, expectedComplexUnit(), output.String())
}

func expectedComplexUnit() string {

	return `[Unit]
Description=The Kolide Launcher
After=network.service syslog.service

[Service]
Environment=KOLIDE_LAUNCHER_ENROLL_SECRET_PATH=/etc/kolide-app/secret
Environment=KOLIDE_LAUNCHER_HOSTNAME=device.kolide.com:443
Environment=KOLIDE_LAUNCHER_OSQUERYD_PATH=/usr/local/kolide-app/bin/osqueryd
Environment=KOLIDE_LAUNCHER_ROOT_DIRECTORY=/var/kolide-app/device.kolide.com-443
Environment=KOLIDE_LAUNCHER_UPDATE_CHANNEL=nightly
ExecStart=/usr/local/kolide-app/bin/launcher \
--autoupdate \
--with_initial_runner
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target`

}
