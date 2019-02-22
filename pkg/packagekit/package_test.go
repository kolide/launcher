package packagekit

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/kolide/kit/env"
	"github.com/stretchr/testify/require"
)

func TestPackageTrivial(t *testing.T) {
	t.Parallel()
	// This test won't work in CI. It's got dependencies on docker, as
	// well as the osx packaging tools. So, skip it unless we've
	// explicitly asked to run it.
	if !env.Bool("CI_TEST_PACKAGING", false) {
		t.Skip("No packaging tools")
	}

	inputDir, err := ioutil.TempDir("", "packaging-input")
	require.NoError(t, err)

	po := &PackageOptions{
		Name:       "test-empty",
		Version:    "0.0.0",
		Root:       inputDir,
		SigningKey: "Developer ID Installer: Kolide Inc (YZ3EM74M78)",
	}

	err = PackageFPM(context.TODO(), ioutil.Discard, po, AsTar())
	require.NoError(t, err)

	err = PackageFPM(context.TODO(), ioutil.Discard, po, AsDeb())
	require.NoError(t, err)

	err = PackageFPM(context.TODO(), ioutil.Discard, po, AsRPM())
	require.NoError(t, err)

	err = PackagePkg(context.TODO(), ioutil.Discard, po)
	require.NoError(t, err)

}
