package launchd

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderEmpty(t *testing.T) {
	err := Render(ioutil.Discard, "empty")
	require.NoError(t, err)
}

func TestRenderComplex(t *testing.T) {
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

	var output bytes.Buffer

	err := Render(&output, "complex", WithEnv(env), WithFlags(flags))
	require.NoError(t, err)

	//require.True(t, strings.Contains(output.String(), expectedFlags))

}
