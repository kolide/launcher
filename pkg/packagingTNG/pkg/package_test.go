package pkg

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageTrivial(t *testing.T) {

	inputDir, err := ioutil.TempDir("/tmp", "packaging-input")
	require.NoError(t, err)

	err = Package(ioutil.Discard, "empty", inputDir)
	require.NoError(t, err)

}
