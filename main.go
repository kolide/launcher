package main

import (
	"flag"
	"fmt"
)

var (
	// version is the version of osquery that a user is requesting to run
	version = flag.String("version", "stable", "The version of osqueryd to run")
	// flagfile is the gflags flagfile to run osqueryd with
	flagfile = flag.String("flagfile", "", "The path to the osqueryd flag file")
)

// OsqueryVersion is a typed string to make passing around osquery version more
// strongly typed
type OsqueryVersion string

// Manifest is a type which represents a set of information about available
// osquery versions that are available to be managed
type Manifest struct {
	Versions []OsqueryVersion
	Stable   OsqueryVersion
}

// fetchManifest retrieves a manifest of available osquery versions from a
// remote server and unmarshals it into a Manifest struct
func fetchManifest() (Manifest, error) {
	// TODO fetch this data structure from a remote API
	return Manifest{
		Versions: []OsqueryVersion{
			"1.4.0",
			"1.4.1",
			"1.4.2",
		},
		Stable: "1.4.2",
	}, nil
}

func main() {
	flag.Parse()

	// Analyze the command inputs to determine what the requested actions are
	// TODO

	// Analyze the current state of the operating system to determine what actions
	// may be necessary
	// TODO

	// Fetch a manifest of the available osquery versions
	manifest, err := fetchManifest()
	if err != nil {
		fmt.Println("Error retrieving a remote manifest:", err)
	}
	_ = manifest

	// Validate that the requested options are possible given the manifest
	// TODO

	// Resolve any unresolved objectives (download new versions of osquery, etc)
	// TODO

	// Launch osqueryd with the supplied options
	// TODO
}
