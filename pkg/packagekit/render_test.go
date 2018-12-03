package packagekit

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderEmpty(t *testing.T) {
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
