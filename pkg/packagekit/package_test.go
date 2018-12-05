package packagekit

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageTrivial(t *testing.T) {

	inputDir, err := ioutil.TempDir("/tmp", "packaging-input")
	require.NoError(t, err)

	po := &PackageOptions{
		Name:    "test-empty",
		Version: "0.0.0",
		Root:    inputDir,
	}

	err = PackageDeb(ioutil.Discard, po)
	require.NoError(t, err)

	err = PackageRPM(ioutil.Discard, po)
	require.NoError(t, err)

	err = PackagePkg(ioutil.Discard, po)
	require.NoError(t, err)

	err = PackagePkg(ioutil.Discard, po, WithSigningKey("Developer ID Installer: Kolide Inc (YZ3EM74M78)"))
	require.NoError(t, err)

}
