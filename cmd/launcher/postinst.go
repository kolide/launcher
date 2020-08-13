package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/kolide/kit/version"
	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

type InstallerInfo struct {
	XMLName struct{} `xml:"InstallerInfo" json:"-"`

	Identifier   string `json:"identifier,omitempty"`
	InstallerId  string `json:"installer_id,omitempty"`
	DownloadPath string `json:"download_path,omitempty"`
	DownloadFile string `json:"download_file,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	Version      string `json:"version,omitempty"`
	User         string `json:"user,omitempty"`
}

func runPostinst(args []string) error {
	hackLog, err := os.Create("/Users/seph/Downloads/postinst.log")
	if err != nil {
		return err
	}
	defer hackLog.Close()

	fmt.Fprintf(hackLog, "postinst begin at %s\n", time.Now().Format(time.RFC3339))
	hackLog.Sync()

	if cwd, err := os.Getwd(); err != nil {
		fmt.Fprintf(hackLog, "cwd: %v\n", err)
	} else {
		fmt.Fprintf(hackLog, "cwd: %s\n", cwd)
	}
	hackLog.Sync()

	fmt.Fprintf(hackLog, "env:\n")
	for _, e := range os.Environ() {
		fmt.Fprintf(hackLog, "  %s\n", e)
	}

	flagset := flag.NewFlagSet("launcher postinstall", flag.ExitOnError)
	flagset.Usage = commandUsage(flagset, "launcher postinstall")

	var (
		flConfig        = flagset.String("config", "", "config file to parse options from (optional)")
		flDebug         = flagset.Bool("debug", false, "Whether or not debug logging is enabled (default: false)")
		flIdentifier    = flagset.String("identifier", "", "Launcher Identifier")
		flInstallerPath = flagset.String("installer_path", "", "Path to the installer")
		flTargetFile    = flagset.String("target", "", "Location of info file")
	)

	if err := ff.Parse(flagset, args,
		ff.WithConfigFileFlag("config"),
		ff.WithIgnoreUndefined(true), // covers unknowns from the config file _only_
		ff.WithConfigFileParser(ff.PlainParser),
	); err != nil {
		fmt.Fprintf(hackLog, "error with flags: %v\n", err)
		hackLog.Sync()
		return err
	}

	fmt.Fprintf(hackLog, "flags ok\n")
	hackLog.Sync()

	_ = flDebug

	// Need to be able to find a target file. We can do this by guessing
	// based on the config file, or something explicit can be provided.
	if *flTargetFile == "" && *flConfig != "" {
		*flTargetFile = filepath.Join(filepath.Dir(*flConfig), "postinst-test.json")
	}

	if *flTargetFile == "" {
		return errors.New("Unable to determine target file. Please use a `config` or `target` option")
	}

	fmt.Fprintf(hackLog, "targetfile: %s\n", *flTargetFile)
	hackLog.Sync()

	// If identifier is unset, autodetect from the config path. We do
	// this by traversing elements upwards until we get something that
	// looks reasonable. This is not very reliable.
	if *flIdentifier == "" && *flConfig != "" {
		// never iterate more than 20 times
		for i := 0; i < 20; i++ {
			*flIdentifier = filepath.Base(filepath.Dir(*flConfig))
			if *flIdentifier != "" &&
				*flIdentifier != "." &&
				*flIdentifier != ".." &&
				*flIdentifier != "conf" {
				break
			}
		}
	}

	installerInfo := &InstallerInfo{
		DownloadFile: filepath.Base(*flInstallerPath),
		DownloadPath: *flInstallerPath,
		Identifier:   *flIdentifier,
		Timestamp:    time.Now().Format(time.RFC3339),
		User:         os.Getenv("USERNAME"),
		Version:      version.Version().Version,
	}

	infoBytes, err := json.MarshalIndent(installerInfo, "", "  ")
	if err != nil {
		return errors.Wrap(err, "json marshal")
	}

	fmt.Printf("Writing to %s\n", *flTargetFile)
	if err := ioutil.WriteFile(*flTargetFile, infoBytes, 0644); err != nil {
		fmt.Fprintf(hackLog, "error with writefile: %v\n", err)
		hackLog.Sync()
		return err
	}
	return nil
}
