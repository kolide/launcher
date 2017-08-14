package packaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTenantName(t *testing.T) {
	require.Equal(t, TenantName(100001), "dababi")
}
