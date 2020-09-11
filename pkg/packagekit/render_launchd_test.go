// +build !windows

package packagekit

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"howett.net/plist"
)

func TestRenderLaunchdEmpty(t *testing.T) {
	t.Parallel()

	expectedOutputStrings := []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
	}

	var output bytes.Buffer
	err := RenderLaunchd(context.TODO(), &output, emptyInitOptions())
	require.NoError(t, err)
	require.True(t, len(output.String()) > 100)

	for _, s := range expectedOutputStrings {
		require.Contains(t, output.String(), s)
	}

}

func TestRenderLaunchdComplex(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := RenderLaunchd(context.TODO(), &output, complexInitOptions())
	require.NoError(t, err)

	expectedData, err := expectedComplex()
	require.NoError(t, err)

	var generatedData launchdOptions
	_, err = plist.Unmarshal(output.Bytes(), &generatedData)
	require.NoError(t, err)

	require.True(t, len(output.String()) > 1000)
	require.Equal(t, expectedData, generatedData)
}

// expectedComplex returns the expected data. It uses
// `DHowett/go-plist` so we can cross-check our encoder.
func expectedComplex() (launchdOptions, error) {
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
    <key>RunAtLoad</key>
    <false/>
    <key>StandardErrorPath</key>
    <string>/var/log/kolide-app/launcher-stderr.log</string>
    <key>StandardOutPath</key>
    <string>/var/log/kolide-app/launcher-stdout.log</string>
  </dict>
</plist>`

	// TODO this should be unmarshaled into a generic struct, so we can
	// capture any missing fields.
	var expectedData launchdOptions
	_, err := plist.Unmarshal([]byte(expected), &expectedData)
	return expectedData, err
}
