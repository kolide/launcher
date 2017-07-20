package packaging

import (
	"html/template"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderLaunchDaemonTemplate(t *testing.T) {
	compiledTemplate, err := template.New("LaunchDaemon").Parse(launchDaemonTemplate)
	require.NoError(t, err)
	options := &launchDaemonTemplateOptions{
		KolideURL: "kolide.com",
	}
	require.NoError(t, compiledTemplate.ExecuteTemplate(os.Stdout, "LaunchDaemon", options))
}
