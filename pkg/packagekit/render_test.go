package packagekit

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderEmpty(t *testing.T) {
	t.Parallel()
	var err error

	initOptions := &InitOptions{
		Name:        "empty",
		Description: "Empty Example",
	}

	err = RenderLaunchd(ioutil.Discard, initOptions)
	require.NoError(t, err)

	err = RenderSystemd(ioutil.Discard, initOptions)
	require.NoError(t, err)

}

func TestRenderComplex(t *testing.T) {
	t.Parallel()
	var err error

	env := map[string]string{
		"FOO": "bar",
		"BAR": "qux",
	}

	flags := []string{
		"--debug",
		"--hello", "world",
		"--array", "one",
		"--array=two",
	}

	initOptions := &InitOptions{
		Name:        "complex",
		Description: "Complex Example",
		Environment: env,
		Flags:       flags,
	}

	var output bytes.Buffer

	err = RenderLaunchd(&output, initOptions)
	require.NoError(t, err)

	err = RenderSystemd(&output, initOptions)
	require.NoError(t, err)

	//require.True(t, strings.Contains(output.String(), expectedFlags))

}

// TestRenderLauncher tests rendering a startup file exactly as we have it
func TestRenderLauncherSystemd(t *testing.T) {
	t.Parallel()

	launcherFlags := []string{
		"--root_directory=/var/kolide-app/device.kolide.com-443",
		"--hostname=device.kolide.com:443",
		"--enroll_secret_path=/etc/kolide-app/secret",
		"--with_initial_runner",
		"--autoupdate",
		"--update_channel=nightly",
		"--osqueryd_path=/usr/local/kolide-app/bin/osqueryd",
	}

	initOptions := &InitOptions{
		Name:        "launcher",
		Description: "The Kolide Launcher",
		Path:        "/usr/local/kolide-app/bin/launcher",
		Flags:       launcherFlags,
	}

	var output bytes.Buffer
	err := RenderSystemd(&output, initOptions)
	require.NoError(t, err)

	expected := `[Unit]
Description=The Kolide Launcher
After=network.service syslog.service

[Service]
ExecStart=/usr/local/kolide-app/bin/launcher \
--root_directory=/var/kolide-app/device.kolide.com-443 \
--hostname=device.kolide.com:443 \
--enroll_secret_path=/etc/kolide-app/secret \
--with_initial_runner \
--autoupdate \
--update_channel=nightly \
--osqueryd_path=/usr/local/kolide-app/bin/osqueryd
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target`

	require.Equal(t, expected, output.String())

}
