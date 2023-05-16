package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/log/checkpoint"

	"github.com/fatih/color"
	"golang.org/x/exp/slices"
)

var (
	cRunning = color.New(color.FgCyan)
	cHeader  = color.New(color.FgHiWhite).Add(color.Bold)
	cWarn    = color.New(color.FgHiYellow)

	pass = color.New(color.FgGreen).PrintlnFunc()
	fail = color.New(color.FgRed).Add(color.Bold).PrintlnFunc()

	// Println functions for checkup details
	cInfo = color.New(color.FgWhite)
	info  = func(a ...interface{}) {
		cInfo.Println(fmt.Sprintf("    %s", a...))
	}
	warn = func(a ...interface{}) {
		cWarn.Println(fmt.Sprintf("    %s", a...))
	}
	bad = func(a ...interface{}) {
		cInfo.Println(fmt.Sprintf("❌  %s", a...))
	}
	good = func(a ...interface{}) {
		cInfo.Println(fmt.Sprintf("✅  %s", a...))
	}
)

// checkup is
type checkup struct {
	name   string
	check  func() (string, error)
	failed bool
}

func runDoctor(args []string) error {
	logger := log.With(logutil.NewCLILogger(true), "caller", log.DefaultCaller)
	opts, err := parseOptions(os.Args[2:])
	if err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	flagController := flags.NewFlagController(logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	cRunning.Println("Kolide launcher doctor version:")
	version.PrintFull()
	cRunning.Println("\nRunning Kolide launcher checkups...")

	checkups := []*checkup{
		{
			name: "Check platform",
			check: func() (string, error) {
				return checkupPlatform(runtime.GOOS)
			},
		},
		{
			name: "Directory contents",
			check: func() (string, error) {
				return checkupRootDir(getFilepaths(k.RootDirectory(), "*"))
			},
		},
		{
			name: "Check communication with Kolide",
			check: func() (string, error) {
				return checkupConnectivity(logger, k)
			},
		},
		{
			name: "Check version",
			check: func() (string, error) {
				return checkupVersion(version.Version())
			},
		},
		{
			name: "Check config file",
			check: func() (string, error) {
				// "/var/kolide-k2/k2device-preprod.kolide.com/launcher.flags"
				filepaths := getFilepaths("/etc/kolide-k2/", "launcher.flags")
				if filepaths != nil && len(filepaths) > 0 {
					return checkupConfigFile(filepaths[0])
				}
				return "", nil // TODO
			},
		},
		{
			name: "Check logs",
			check: func() (string, error) {
				return checkupLogFiles(getFilepaths(k.RootDirectory(), "debug*"))
			},
		},
	}

	failedCheckups := []*checkup{}

	// Sequentially run all of the checkups
	for _, c := range checkups {
		c.run()
	}

	if len(failedCheckups) > 0 {
		fail("\nSome checkups failed:")

		for _, c := range failedCheckups {
			fail("%s\n", c.name)
		}
	} else {
		pass("\nAll checkups passed! Your Kolide launcher is healthy.")
	}

	return nil
}

func (c *checkup) run() {
	if c.check == nil {
	}

	cRunning.Printf("\nRunning checkup: ")
	cHeader.Printf("%s\n", c.name)

	result, err := c.check()
	if err != nil {
		info(result)
		bad(err)
		fail("𐄂 Checkup failed!")
	} else {
		good(result)
		pass("✔ Checkup passed!")
	}
}

func checkupPlatform(os string) (string, error) {
	if slices.Contains([]string{"windows", "darwin", "linux"}, os) {
		return fmt.Sprintf("Platform: %s", os), nil
	}
	return "", fmt.Errorf("Unsupported platform: %s", os)
}

type launcherFile struct {
	name  string
	found bool
}

func checkupRootDir(filepaths []string) (string, error) {
	importantFiles := []*launcherFile{
		{
			name: "debug.json",
		},
		{
			name: "launcher.db",
		},
		{
			name: "osquery.db",
		},
	}

	if filepaths != nil && len(filepaths) > 0 {
		for _, fp := range filepaths {
			for _, f := range importantFiles {
				if filepath.Base(fp) == f.name {
					f.found = true
				}
			}
		}
	}

	var failures int
	for _, f := range importantFiles {
		if f.found {
			good(f.name)
		} else {
			bad(f.name)
			failures = failures + 1
		}
	}

	if failures == 0 {
		return "Root directory files found", nil
	}

	return "", fmt.Errorf("%d root directory files not found", failures)
}

func checkupConnectivity(logger log.Logger, k types.Knapsack) (string, error) {
	checkpointer := checkpoint.New(logger, k)

	connections := checkpointer.Connections()
	for k, v := range connections {
		if v == "successful tcp connection" {
			good(fmt.Sprintf("%s - %s", k, v))
		} else {
			bad(fmt.Sprintf("%s - %s", k, v))
		}
	}

	ipLookups := checkpointer.IpLookups()
	for k, v := range ipLookups {
		valStrSlice, ok := v.([]string)
		if !ok || len(valStrSlice) == 0 {
			bad(fmt.Sprintf("%s - %s", k, valStrSlice))
		} else {
			good(fmt.Sprintf("%s - %s", k, valStrSlice))
		}
	}

	return "Successfully communicated with Kolide", nil
}

func checkupVersion(v version.Info) (string, error) {
	info(fmt.Sprintf("Current version: %s", v.Version))
	// TODO: Query TUF for latest available version for this launcher instance
	warn(fmt.Sprintf("Target version: %s", "Unknown"))

	// TODO: Choose failure based on current >= target
	if true {
		return "Up to date!", nil
	}

	return "", fmt.Errorf("launcher is out of date")
}

func checkupConfigFile(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("No config file found")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		info(scanner.Text())
	}

	return "Config file found", nil
}

func checkupLogFiles(filepaths []string) (string, error) {
	var foundCurrentLogFile bool
	for _, f := range filepaths {
		filename := filepath.Base(f)
		info(filename)

		if filename == "debug.json" {
			foundCurrentLogFile = true

			fi, err := os.Stat(f)
			if err == nil {
				info("")
				info(fmt.Sprintf("Most recent log file: %s", filename))
				info(fmt.Sprintf("Latest modification: %s", fi.ModTime().String()))
				info(fmt.Sprintf("File size (B): %d", fi.Size()))
			}
		}
	}

	if foundCurrentLogFile {
		return "Log file found", nil
	}
	return "", fmt.Errorf("No log file found")
}

func getFilepaths(elem ...string) []string {
	fileGlob := filepath.Join(elem...)
	filepaths, err := filepath.Glob(fileGlob)

	if err == nil && len(filepaths) > 0 {
		return filepaths
	}

	return nil
}
