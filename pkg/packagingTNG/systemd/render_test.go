package systemd

import (
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
	err := Render(ioutil.Discard, "complex", WithEnv(env), WithFlags(flags))
	require.NoError(t, err)
}
