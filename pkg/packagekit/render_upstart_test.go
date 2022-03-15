//go:build !windows
// +build !windows

// These tests fail on windows, due to what looks like line ending
// issues. Since we're not going to be building upstart on windows, just skip it.
package packagekit

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderUpstartEmpty(t *testing.T) {
	t.Parallel()

	expectedOutputStrings := []string{
		`#!upstart`,
		`# Name: empty`,
	}

	var output bytes.Buffer
	err := RenderUpstart(context.TODO(), &output, emptyInitOptions())
	require.NoError(t, err)
	require.True(t, len(output.String()) > 100)

	for _, s := range expectedOutputStrings {
		require.Contains(t, output.String(), s)
	}

}

func TestRenderUpstartComplex(t *testing.T) {
	t.Parallel()

	expectedOutputStrings := []string{
		`#!upstart`,
		`# Name: launcher`,
	}

	var output bytes.Buffer
	err := RenderUpstart(context.TODO(), &output, complexInitOptions())
	require.NoError(t, err)
	require.True(t, len(output.String()) > 100)

	for _, s := range expectedOutputStrings {
		require.Contains(t, output.String(), s)
	}

}

func TestRenderUpstartOptions(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		uOpts             []UpstartOption
		expectedStrings   []string
		unexpectedStrings []string
	}{
		{
			uOpts: []UpstartOption{WithPreStartScript([]string{"hello", "world"})},
			expectedStrings: []string{
				"pre-start script\nhello\nworld\nend script",
			},
			unexpectedStrings: []string{
				"pre-stop script\nhello\nworld\nend script",
				"post-start script\nhello\nworld\nend script",
			},
		},
		{
			uOpts: []UpstartOption{WithPostStartScript([]string{"hello", "world"})},
			expectedStrings: []string{
				"post-start script\nhello\nworld\nend script",
			},
			unexpectedStrings: []string{
				"pre-stop script\nhello\nworld\nend script",
				"pre-start script\nhello\nworld\nend script",
			},
		},
		{
			uOpts: []UpstartOption{WithPreStopScript([]string{"hello", "world"})},
			expectedStrings: []string{
				"pre-stop script\nhello\nworld\nend script",
			},
			unexpectedStrings: []string{
				"pre-start script\nhello\nworld\nend script",
				"post-start script\nhello\nworld\nend script",
			},
		},
		{
			uOpts: []UpstartOption{WithExpect("fork")},
			expectedStrings: []string{
				"expect fork",
			},
		},
		{
			uOpts: []UpstartOption{WithUpstartFlavor("")},
			expectedStrings: []string{
				"console log",
				"start on net-device-up",
			},
		},
		{
			uOpts: []UpstartOption{WithUpstartFlavor("amazon-ami")},
			expectedStrings: []string{
				"started network",
			},
			unexpectedStrings: []string{
				"console log",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			err := RenderUpstart(context.TODO(), &output, emptyInitOptions(), tt.uOpts...)
			require.NoError(t, err)
			require.True(t, len(output.String()) > 100)

			for _, s := range tt.expectedStrings {
				require.Contains(t, output.String(), s)
			}

			for _, s := range tt.unexpectedStrings {
				require.NotContains(t, output.String(), s)
			}
		})
	}
}
