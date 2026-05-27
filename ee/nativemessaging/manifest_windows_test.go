//go:build windows

package nativemessaging

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

func Test_registerManifestFileLocation(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("some", "test", "dir", "nmh-manifest.json")
	testRegistryKey := `SOFTWARE\Kolide\Launcher\Test_registerManifestFileLocation\Google\Chrome\NativeMessagingHosts\` + nativeMessagingHostName

	require.NoError(t, registerManifestFileLocation(manifestPath, testRegistryKey))

	manifestLocationKey, err := registry.OpenKey(registry.LOCAL_MACHINE, testRegistryKey, registry.ALL_ACCESS)
	require.NoError(t, err)
	t.Cleanup(func() {
		manifestLocationKey.Close()
	})
	currentValue, _, err := manifestLocationKey.GetStringValue("")
	require.NoError(t, err)
	require.Equal(t, manifestPath, currentValue)
}
