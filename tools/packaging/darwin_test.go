package packaging

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderLaunchDaemonTemplate(t *testing.T) {
	require.NoError(t, renderLaunchDaemon(os.Stdout, &launchDaemonTemplateOptions{KolideURL: "kolide.com"}))
}
