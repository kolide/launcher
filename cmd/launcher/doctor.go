package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/checkpoint"
	"github.com/peterbourgon/ff/v3"
	"github.com/shirou/gopsutil/v3/process"

	"golang.org/x/exp/slices"
)

var (
	doctorWriter io.Writer

	// Command line colors
	cyanText   *color.Color
	headerText *color.Color
	yellowText *color.Color
	whiteText  *color.Color
	greenText  *color.Color
	redText    *color.Color

	// Printf functions
	cyan   func(format string, a ...interface{})
	header func(format string, a ...interface{})

	// Println functions for checkup details
	green func(a ...interface{})
	red   func(a ...interface{})

	// Indented output for checkup results
	info func(a ...interface{})
	warn func(a ...interface{})
	fail func(a ...interface{})
	pass func(a ...interface{})
)

func configureOutput(w io.Writer) {
	// Set the writer to be used for doctor output
	writer := tabwriter.NewWriter(w, 0, 8, 1, '\t', tabwriter.AlignRight)
	doctorWriter = writer

	// Command line colors
	cyanText = color.New(color.FgCyan, color.BgBlack)
	headerText = color.New(color.Bold, color.FgHiWhite, color.BgBlack)
	yellowText = color.New(color.FgHiYellow, color.BgBlack)
	whiteText = color.New(color.FgWhite, color.BgBlack)
	greenText = color.New(color.FgGreen, color.BgBlack)
	redText = color.New(color.Bold, color.FgRed, color.BgBlack)

	// Printf functions
	cyan = func(format string, a ...interface{}) {
		cyanText.Fprintf(doctorWriter, format, a...)
	}
	header = func(format string, a ...interface{}) {
		headerText.Fprintf(doctorWriter, format, a...)
	}

	// Println functions for checkup details
	green = func(a ...interface{}) {
		greenText.Fprintln(doctorWriter, a...)
	}
	red = func(a ...interface{}) {
		redText.Fprintln(doctorWriter, a...)
	}

	// Indented output for checkup results
	info = func(a ...interface{}) {
		whiteText.FprintlnFunc()(doctorWriter, fmt.Sprintf("\t%s", a...))
	}
	warn = func(a ...interface{}) {
		yellowText.FprintlnFunc()(doctorWriter, fmt.Sprintf("\t%s", a...))
	}
	fail = func(a ...interface{}) {
		whiteText.FprintlnFunc()(doctorWriter, fmt.Sprintf("❌\t%s", a...))
	}
	pass = func(a ...interface{}) {
		whiteText.FprintlnFunc()(doctorWriter, fmt.Sprintf("✅\t%s", a...))
	}
}

// checkup encapsulates a launcher health checkup
type checkup struct {
	name  string
	check func() (string, error)
}

func runDoctor(args []string) error {
	// Doctor assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	defaultKolideHosted = true
	defaultAutoupdate = true
	setDefaultPaths()

	opts, err := parseOptions("doctor", os.Args[2:])
	if err != nil {
		return err
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	logger := log.With(logutil.NewCLILogger(true), "caller", log.DefaultCaller)
	flagController := flags.NewFlagController(logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil)

	buildAndRunCheckups(logger, k, opts, os.Stdout)

	return nil
}

// buildAndRunCheckups creates a list of checkups and executes them
func buildAndRunCheckups(logger log.Logger, k types.Knapsack, opts *launcher.Options, w io.Writer) {
	configureOutput(w)

	cyan("Kolide launcher doctor version:\n")
	version.PrintFull()
	cyan("\nRunning Kolide launcher checkups...\n")

	checkups := []*checkup{
		{
			name: "Platform",
			check: func() (string, error) {
				return checkupPlatform(runtime.GOOS)
			},
		},
		{
			name: "Architecture",
			check: func() (string, error) {
				return checkupArch(runtime.GOARCH)
			},
		},
		{
			name: "Root directory contents",
			check: func() (string, error) {
				return checkupRootDir(getFilepaths(k.RootDirectory(), "*"))
			},
		},
		{
			name: "Launcher application",
			check: func() (string, error) {
				return checkupAppBinaries(getAppBinaryPaths())
			},
		},
		{
			name: "Osquery",
			check: func() (string, error) {
				return checkupOsquery(opts.UpdateChannel.String(), opts.TufServerURL, opts.OsquerydPath)
			},
		},
		{
			name: "Check communication with Kolide",
			check: func() (string, error) {
				return checkupConnectivity(logger, k)
			},
		},
		{
			name: "Check launcher version",
			check: func() (string, error) {
				return checkupVersion(opts.UpdateChannel.String(), opts.TufServerURL, version.Version())
			},
		},
		{
			name: "Check config file",
			check: func() (string, error) {
				return checkupConfigFile(opts.ConfigFilePath)
			},
		},
		{
			name: "Check logs",
			check: func() (string, error) {
				return checkupLogFiles(getFilepaths(k.RootDirectory(), "debug*"))
			},
		},
		{
			name: "Process report",
			check: func() (string, error) {
				return checkupProcessReport()
			},
		},
	}

	runCheckups(checkups)
}

// runCheckups iterates through the checkups and logs success/failure information
func runCheckups(checkups []*checkup) {
	failedCheckups := []*checkup{}

	// Sequentially run all of the checkups
	for _, c := range checkups {
		err := c.run()
		if err != nil {
			failedCheckups = append(failedCheckups, c)
		}
	}

	if len(failedCheckups) > 0 {
		red("\nSome checkups failed:")

		for _, c := range failedCheckups {
			fail(fmt.Sprintf("\t%s\n", c.name))
		}
		return
	}

	green("\nAll checkups passed! Your Kolide launcher is healthy.")
}

// run logs the results of a checkup being run
func (c *checkup) run() error {
	if c.check == nil {
		return errors.New("checkup is nil")
	}

	cyan("\nRunning checkup: ")
	header("%s\n", c.name)

	result, err := c.check()
	if err != nil {
		info(result)
		fail(err)
		red("𐄂\tCheckup failed!")
		return err
	}

	pass(result)
	green("✔\tCheckup passed!")
	return nil
}

// checkupPlatform verifies that the current OS is supported by launcher
func checkupPlatform(os string) (string, error) {
	if slices.Contains([]string{"windows", "darwin", "linux"}, os) {
		return fmt.Sprintf("Platform: %s", os), nil
	}
	return "", fmt.Errorf("Unsupported platform:\t%s", os)
}

// checkupArch verifies that the current architecture is supported by launcher
func checkupArch(arch string) (string, error) {
	if slices.Contains([]string{"386", "amd64", "arm64"}, arch) {
		return fmt.Sprintf("Architecture: %s", arch), nil
	}
	return "", fmt.Errorf("Unsupported architecture:\t%s", arch)
}

type launcherFile struct {
	name  string
	found bool
}

// checkupRootDir tests for the presence of important files in the launcher root directory
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

	return checkupFilesPresent(filepaths, importantFiles)
}

func checkupAppBinaries(filepaths []string) (string, error) {
	importantFiles := []*launcherFile{
		{
			name: "launcher",
		},
	}

	return checkupFilesPresent(filepaths, importantFiles)
}

// checkupOsquery tests for the presence of files important to osquery
func checkupOsquery(updateChannel, tufServerURL, osquerydPath string) (string, error) {
	if osquerydPath == "" {
		return "", fmt.Errorf("osqueryd path unknown")
	}

	_, err := os.Stat(osquerydPath)
	if err != nil {
		return "", fmt.Errorf("osqueryd does not exist")
	}

	osqueryArgs := []string{"--version"}
	cmd := exec.CommandContext(context.TODO(), osquerydPath, osqueryArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error occurred while querying osquery version output %s: err: %w", out, err)
	}

	currentVersion := strings.TrimLeft(string(out), "osqueryd version ")
	currentVersion = strings.TrimRight(currentVersion, "\n")

	info(fmt.Sprintf("Current version:\t%s", currentVersion))

	// Query the TUF repo for what the target version of osquery is
	targetVersion, err := tuf.GetChannelVersionFromTufServer("osqueryd", updateChannel, tufServerURL)
	if err != nil {
		return "", fmt.Errorf("Failed to query TUF server: %w", err)
	}

	info(fmt.Sprintf("Target version:\t%s", targetVersion))

	if targetVersion != currentVersion {
		return "", fmt.Errorf("osqueryd is out of date")
	}
	return "Up to date!", nil
}

func checkupFilesPresent(filepaths []string, importantFiles []*launcherFile) (string, error) {
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
			pass(f.name)
		} else {
			fail(f.name)
			failures = failures + 1
		}
	}

	if failures == 0 {
		return "Files found", nil
	}

	return "", fmt.Errorf("%d files not found", failures)
}

// checkupConnectivity tests connections to Kolide cloud services
func checkupConnectivity(logger log.Logger, k types.Knapsack) (string, error) {
	var failures int
	checkpointer := checkpoint.New(logger, k)
	connections := checkpointer.Connections()
	for k, v := range connections {
		if v != "successful tcp connection" {
			fail(fmt.Sprintf("%s\t%s", k, v))
			failures = failures + 1
			continue
		}
		pass(fmt.Sprintf("%s\t%s", k, v))
	}

	ipLookups := checkpointer.IpLookups()
	for k, v := range ipLookups {
		valStrSlice, ok := v.([]string)
		if !ok || len(valStrSlice) == 0 {
			fail(fmt.Sprintf("%s\t%s", k, valStrSlice))
			failures = failures + 1
			continue
		}
		pass(fmt.Sprintf("%s\t%s", k, valStrSlice))
	}

	notaryVersions, err := checkpointer.NotaryVersions()
	if err != nil {
		fail(fmt.Errorf("could not fetch notary versions: %w", err))
		failures = failures + 1
	}

	for k, v := range notaryVersions {
		// Check for failure if the notary version isn't a parsable integer
		if _, err := strconv.ParseInt(v, 10, 32); err != nil {
			fail(fmt.Sprintf("%s\t%s", k, v))
			failures = failures + 1
			continue
		}
		pass(fmt.Sprintf("%s\t%s", k, v))
	}

	if failures == 0 {
		return "Successfully communicated with Kolide", nil
	}

	return "", fmt.Errorf("%d failures encountered while attempting communication with Kolide", failures)
}

// checkupVersion tests to see if the current launcher version is up to date
func checkupVersion(updateChannel, tufServerURL string, v version.Info) (string, error) {
	info(fmt.Sprintf("Update Channel:\t%s", updateChannel))
	info(fmt.Sprintf("TUF Server:\t%s", tufServerURL))
	info(fmt.Sprintf("Current version:\t%s", v.Version))

	// Query the TUF repo for what the target version of launcher is
	targetVersion, err := tuf.GetChannelVersionFromTufServer("launcher", updateChannel, tufServerURL)
	if err != nil {
		return "", fmt.Errorf("Failed to query TUF server: %w", err)
	}

	info(fmt.Sprintf("Target version:\t%s", targetVersion))

	if targetVersion != v.Version {
		return "", fmt.Errorf("launcher is out of date")
	}
	return "Up to date!", nil
}

// checkupConfigFile tests that the config file is valid and logs it's contents
func checkupConfigFile(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("No config file found")
	}
	defer file.Close()

	// Parse the config file how launcher would
	err = ff.PlainParser(file, func(name, value string) error {
		info(fmt.Sprintf("%s\t%s", name, value))
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("Invalid config file")
	}
	return "Config file found", nil
}

// checkupLogFiles checks to see if expected log files are present
func checkupLogFiles(filepaths []string) (string, error) {
	var foundCurrentLogFile bool
	for _, f := range filepaths {
		filename := filepath.Base(f)
		info(filename)

		if filename != "debug.json" {
			continue
		}

		foundCurrentLogFile = true

		fi, err := os.Stat(f)
		if err != nil {
			continue
		}

		info("")
		info(fmt.Sprintf("Most recent log file:\t%s", filename))
		info(fmt.Sprintf("Latest modification:\t%s", fi.ModTime().String()))
		info(fmt.Sprintf("File size (B):\t%d", fi.Size()))
	}

	if !foundCurrentLogFile {
		return "", fmt.Errorf("No log file found")
	}

	return "Log file found", nil

}

// checkupProcessReport finds processes that look like Kolide launcher/osquery processes
func checkupProcessReport() (string, error) {
	ps, err := process.Processes()
	if err != nil {
		return "", fmt.Errorf("No processes found")
	}

	var foundKolide bool
	for _, p := range ps {
		exe, _ := p.Exe()

		if strings.Contains(strings.ToLower(exe), "kolide") {
			foundKolide = true
			name, _ := p.Name()
			args, _ := p.Cmdline()
			user, _ := p.Username()
			info(fmt.Sprintf("%s\t%d\t%s\t%s", user, p.Pid, name, args))
		}
	}

	if !foundKolide {
		return "", fmt.Errorf("No launcher processes found")
	}
	return "Launcher processes found", nil
}
