package packagekit

// emptyInitOptions() returns a trivial set of init options for
// testing basic template rendering
func emptyInitOptions() *InitOptions {
	initOptions := &InitOptions{
		Name:        "empty",
		Identifier:  "empty",
		Description: "Empty Example",
		Path:        "/dev/null",
	}

	return initOptions
}

// complexInitOptions returns an initOptions for real world style testing
func complexInitOptions() *InitOptions {
	launcherEnv := map[string]string{
		"KOLIDE_LAUNCHER_ROOT_DIRECTORY":     "/var/kolide-app/device.kolide.com-443",
		"KOLIDE_LAUNCHER_HOSTNAME":           "device.kolide.com:443",
		"KOLIDE_LAUNCHER_ENROLL_SECRET_PATH": "/etc/kolide-app/secret",
		"KOLIDE_LAUNCHER_UPDATE_CHANNEL":     "nightly",
		"KOLIDE_LAUNCHER_OSQUERYD_PATH":      "/usr/local/kolide-app/bin/osqueryd",
	}
	launcherFlags := []string{
		"--autoupdate",
		"--with_initial_runner",
	}

	initOptions := &InitOptions{
		Name:        "launcher",
		Description: "The Kolide Launcher",
		Identifier:  "kolide-app",
		Path:        "/usr/local/kolide-app/bin/launcher",
		Flags:       launcherFlags,
		Environment: launcherEnv,
	}

	return initOptions
}
