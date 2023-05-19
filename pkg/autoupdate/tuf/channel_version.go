package tuf

import (
	"fmt"
	"net/http"

	"github.com/kolide/launcher/pkg/agent"
)

// GetChannelVersionFromTufServer returns the tagged version of the given binary for the given release channel.
// It is intended for use in e.g. packaging and `launcher doctor`, where we want to determine the correct
// version tag for the channel but do not necessarily have access to a local TUF repo.
func GetChannelVersionFromTufServer(binary, channel, tufServerUrl string) (string, error) {
	tempTufRepoDir, err := agent.MkdirTemp("temp-tuf")
	if err != nil {
		return "", fmt.Errorf("could not make temporary directory: %w", err)
	}

	tempTufClient, err := initMetadataClient(tempTufRepoDir, tufServerUrl, http.DefaultClient)
	if err != nil {
		return "", fmt.Errorf("could not init metadata client: %w", err)
	}

	targets, err := tempTufClient.Update()
	if err != nil {
		return "", fmt.Errorf("could not update targets: %w", err)
	}

	releaseTarget, _, err := findRelease(autoupdatableBinary(binary), targets, channel)
	if err != nil {
		return "", fmt.Errorf("could not find release: %w", err)
	}

	return versionFromTarget(autoupdatableBinary(binary), releaseTarget), nil
}
