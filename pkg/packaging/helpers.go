package packaging

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/kit/fs"
	"github.com/pkg/errors"
)

/*
func Todo() {

	scriptDir, err := ioutil.TempDir("", "scriptDir")
	if err != nil {
		return prepOut, err
	}
	prepOut.ScriptsRoot = scriptDir
	postinstallFile, err := os.Create(filepath.Join(scriptDir, "postinstall"))
	if err != nil {
		return prepOut, err
	}
	if err := postinstallFile.Chmod(0755); err != nil {
		return prepOut, err
	}
	defer postinstallFile.Close()

	// handle postinstalls
	switch flavor {
	case LaunchD:
		postinstallOpts := &postinstallTemplateOptions{
			LaunchDaemonDirectory: sp.dir,
			LaunchDaemonName:      sp.serviceName,
		}
		if err := renderPostinstall(postinstallFile, postinstallOpts); err != nil {
			return prepOut, err
		}
	case SystemD:
		postInstallLauncherContents := `#!/bin/bash
set -e
systemctl daemon-reload
systemctl enable launcher
systemctl restart launcher`
		fmt.Fprintf(postinstallFile, postInstallLauncherContents)

	case Init:
		postInstallLauncherContents := `#!/bin/bash
sudo service launcher restart`
		fmt.Fprintf(postinstallFile, postInstallLauncherContents)
	}

	return prepOut, nil
}

*/

func rootDirectory(packageRoot, identifier, hostname string, target Target) (string, error) {
	var dir string
	switch target.Platform {
	case Linux, Darwin:
		dir = filepath.Join("/var", identifier, sanitizeHostname(hostname))
	default:
		return "", errors.Errorf("Unknown platform %s", string(target.Platform))
	}
	err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode)
	return dir, errors.Wrapf(err, "create root dir for %s", target.String())
}

func binaryDirectory(packageRoot, identifier string, target Target) (string, error) {
	var dir string
	switch target.Platform {
	case Linux, Darwin:
		dir = filepath.Join("/usr/local", identifier, "bin")
	default:
		return "", errors.Errorf("Unknown platform %s", string(target.Platform))
	}
	err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode)
	return dir, errors.Wrapf(err, "create binary dir for %s", target.String())
}

func configurationDirectory(packageRoot, identifier string, target Target) (string, error) {
	var dir string
	switch target.Platform {
	case Linux, Darwin:
		dir = filepath.Join("/etc", identifier)
	default:
		return "", errors.Errorf("Unknown platform %s", string(target.Platform))
	}

	err := os.MkdirAll(filepath.Join(packageRoot, dir), fs.DirMode)
	return dir, errors.Wrapf(err, "create config dir for %s", target.String())
}

/*

func renderPostinstall(w io.Writer, options *postinstallTemplateOptions) error {
	postinstallTemplate := `#!/bin/bash

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop {{.LaunchDaemonName}}

sleep 5

/bin/launchctl unload {{.LaunchDaemonDirectory}}/{{.LaunchDaemonName}}.plist
/bin/launchctl load {{.LaunchDaemonDirectory}}/{{.LaunchDaemonName}}.plist`
	t, err := template.New("postinstall").Parse(postinstallTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse postinstall template")
	}
	return t.ExecuteTemplate(w, "postinstall", options)
}

*/

// sanitizeHostname will replace any ":" characters in a given hostname with "-"
// This is useful because ":" is not a valid character for file paths.
func sanitizeHostname(hostname string) string {
	return strings.Replace(hostname, ":", "-", -1)
}
