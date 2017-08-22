package packaging

import (
	"os"
	"os/exec"
)

// GsutilRsync copies a local directory of files to a specified bucket URI
func GsutilRsync(source, bucketURI string) error {
	cmd := exec.Command("gsutil", "-m", "rsync", "-d", "-r", source, bucketURI)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SetGCPProject will set the local GCP project to the supplied project name
func SetGCPProject(project string) error {
	cmd := exec.Command("gcloud", "config", "set", "project", project)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
