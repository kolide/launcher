//go:build windows

package nativemessaging

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

func Test_registerManifestFileLocation_deregisterManifestFileLocation(t *testing.T) {
	t.Parallel()

	hostName := nativeMessagingHostName("kolide-test-k2")
	manifestPath := filepath.Join("some", "test", "dir", "nmh-manifest.json")
	testRegistryKey := `SOFTWARE\Kolide\Launcher\Test_registerManifestFileLocation\Google\Chrome\NativeMessagingHosts\` + hostName

	// Test registration
	require.NoError(t, registerManifestFileLocation(manifestPath, testRegistryKey))

	manifestLocationKey, err := registry.OpenKey(registry.LOCAL_MACHINE, testRegistryKey, registry.ALL_ACCESS)
	require.NoError(t, err)
	t.Cleanup(func() {
		manifestLocationKey.Close()
	})
	currentValue, _, err := manifestLocationKey.GetStringValue("")
	require.NoError(t, err)
	require.Equal(t, manifestPath, currentValue)

	// Test re-registration
	require.NoError(t, registerManifestFileLocation(manifestPath, testRegistryKey))

	// Test deregistration
	require.NoError(t, deregisterManifestFileLocation(testRegistryKey))
	_, err = registry.OpenKey(registry.LOCAL_MACHINE, testRegistryKey, registry.ALL_ACCESS)
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist))
}
