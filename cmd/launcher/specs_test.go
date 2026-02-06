package main

import (
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_runSpecs(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{})
	require.NoError(t, err)
}

func Test_runSpecs_debugFlag(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{"-debug"})
	require.NoError(t, err)
}

func Test_runSpecs_errorOnMissingFlag(t *testing.T) {
	t.Parallel()

	ms := multislogger.New()
	err := runSpecs(ms, []string{"-error-on-missing"})
	require.NoError(t, err)
}
