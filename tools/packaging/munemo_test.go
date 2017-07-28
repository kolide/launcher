package packaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewMunemo(t *testing.T) {
	require.Equal(t, Munemo(100001), "dababi")
}
