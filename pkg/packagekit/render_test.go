package packagekit

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"howett.net/plist"
)

func TestRenderEmpty(t *testing.T) {
	t.Parallel()

	initOptions := &InitOptions{
		Name:        "empty",
		Identifier:  "empty",
		Description: "Empty Example",
		Path:        "/dev/null",
	}

	var tests = []struct {
		renderFunc func(io.Writer, *InitOptions) error
		outSize    int
	}{
		{
			renderFunc: RenderLaunchd,
			outSize:    100,
		},
		{
			renderFunc: RenderSystemd,
			outSize:    100,
		},
		{
			renderFunc: RenderInit,
			outSize:    100,
		},
	}

	for _, tt := range tests {
		var output bytes.Buffer
		err := tt.renderFunc(&output, initOptions)
		require.NoError(t, err)
		require.True(t, len(output.String()) > tt.outSize)
	}
}

func TestRenderComplex(t *testing.T) {
	t.Parallel()

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
		Identifier:  "complex",
		Description: "Complex Example",
		Environment: env,
		Flags:       flags,
		Path:        "/usr/bin/true",
	}

	var tests = []struct {
		renderFunc func(io.Writer, *InitOptions) error
		outSize    int
	}{
		{
			renderFunc: RenderLaunchd,
			outSize:    200,
		},
		{
			renderFunc: RenderSystemd,
			outSize:    200,
		},
		{
			renderFunc: RenderInit,
			outSize:    200,
		},
	}

	for _, tt := range tests {
		var output bytes.Buffer
		err := tt.renderFunc(&output, initOptions)
		require.NoError(t, err)
		require.True(t, len(output.String()) > tt.outSize)
		// TODO Check some of the rendered content
	}

}

// TestRenderLauncher tests rendering a startup file exactly as we have it
func TestRenderLauncherSystemd(t *testing.T) {
	t.Parallel()

	launcherFlags := []string{}

	initOptions := &InitOptions{
		Name:        "launcher",
		Description: "The Kolide Launcher",
		Identifier:  "kolide-app",
		Path:        "/usr/local/kolide-app/bin/launcher",
		Flags:       launcherFlags,
	}

	initOptions.Flags = append(initOptions.Flags,
		"--root_directory=/var/kolide-app/device.kolide.com-443",
		"--hostname=device.kolide.com:443",
		"--enroll_secret_path=/etc/kolide-app/secret",
		"--with_initial_runner",
		"--autoupdate",
		"--update_channel=nightly",
		"--osqueryd_path=/usr/local/kolide-app/bin/osqueryd",
	)

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

// TestRenderLauncher tests rendering a startup file exactly as we have it
func TestRenderLauncherLaunchd(t *testing.T) {
	t.Parallel()

	launcherEnv := map[string]string{
		"KOLIDE_LAUNCHER_ROOT_DIRECTORY":     "/var/kolide-app/device.kolide.com-443",
		"KOLIDE_LAUNCHER_HOSTNAME":           "device.kolide.com:443",
		"KOLIDE_LAUNCHER_ENROLL_SECRET_PATH": "/etc/kolide-app/secret",
		"KOLIDE_LAUNCHER_UPDATE_CHANNEL":     "nightly",
		"KOLIDE_LAUNCHER_OSQUERYD_PATH":      "/usr/local/kolide-app/bin/osqueryd",
	}
	launcherFlags := []string{
		"--autoupdate",
		"--with_initial_runner",
	}

	initOptions := &InitOptions{
		Name:        "launcher",
		Description: "The Kolide Launcher",
		Identifier:  "kolide-app",
		Path:        "/usr/local/kolide-app/bin/launcher",
		Flags:       launcherFlags,
		Environment: launcherEnv,
	}

	var output bytes.Buffer
	err := RenderLaunchd(&output, initOptions)
	require.NoError(t, err)

	// Now, let's check that the content matches. We're doing this with
	// `DHowett/go-plist` so we can cross-check our encoder.

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>com.kolide-app.launcher</string>
    <key>EnvironmentVariables</key>
    <dict>
      <key>KOLIDE_LAUNCHER_ROOT_DIRECTORY</key>
      <string>/var/kolide-app/device.kolide.com-443</string>
      <key>KOLIDE_LAUNCHER_HOSTNAME</key>
      <string>device.kolide.com:443</string>
      <key>KOLIDE_LAUNCHER_ENROLL_SECRET_PATH</key>
      <string>/etc/kolide-app/secret</string>
      <key>KOLIDE_LAUNCHER_OSQUERYD_PATH</key>
      <string>/usr/local/kolide-app/bin/osqueryd</string>
      <key>KOLIDE_LAUNCHER_UPDATE_CHANNEL</key>
      <string>nightly</string>
    </dict>
    <key>KeepAlive</key>
    <dict>
      <key>PathState</key>
      <dict>
        <key>/etc/kolide-app/secret</key>
        <true/>
      </dict>
    </dict>
    <key>ThrottleInterval</key>
    <integer>60</integer>
    <key>ProgramArguments</key>
    <array>
      <string>/usr/local/kolide-app/bin/launcher</string>
      <string>--autoupdate</string>
      <string>--with_initial_runner</string>
    </array>
    <key>StandardErrorPath</key>
    <string>/var/log/kolide-app/launcher-stderr.log</string>
    <key>StandardOutPath</key>
    <string>/var/log/kolide-app/launcher-stdout.log</string>
  </dict>
</plist>`

	// TODO this should be unmarshaled into a generic struct, so we can
	// capture any missing fields.
	var expectedData launchdOptions
	_, err = plist.Unmarshal([]byte(expected), &expectedData)
	require.NoError(t, err)

	var generatedData launchdOptions
	_, err = plist.Unmarshal(output.Bytes(), &generatedData)
	require.NoError(t, err)

	require.True(t, len(output.String()) > 1000)
	require.Equal(t, expectedData, generatedData)

}
