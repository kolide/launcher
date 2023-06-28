package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/cmd/launcher/diag"
	"github.com/kolide/launcher/cmd/launcher/diag/checkups"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/launcher"
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

	cyan("\nRunning Kolide launcher checkups...\n")

	checkups := []diag.CheckupRunner{
		&checkups.Arch{},
		&checkups.Platform{},
		&checkups.RootDirectory{Filepaths: getFilepaths(k.RootDirectory(), "*")},
		&checkups.Binaries{Filepaths: getAppBinaryPaths()},
		&checkups.Osquery{UpdateChannel: opts.UpdateChannel.String(), TufServerURL: opts.TufServerURL, OsquerydPath: opts.OsquerydPath},
		&checkups.Connectivity{Logger: logger, K: k},
		&checkups.Version{UpdateChannel: opts.UpdateChannel.String(), TufServerURL: opts.TufServerURL, Version: version.Version()},
		&checkups.Config{Filepath: opts.ConfigFilePath},
		&checkups.Logs{Filepaths: getFilepaths(k.RootDirectory(), "debug*")},
		&checkups.Processes{},
	}

	runCheckups(checkups)
}

// runCheckups iterates through the checkups and logs success/failure information
func runCheckups(checkups []diag.CheckupRunner) {
	failedCheckups := []diag.CheckupRunner{}

	// Sequentially run all of the checkups
	for _, c := range checkups {
		err := runCheckup(c)
		if err != nil {
			failedCheckups = append(failedCheckups, c)
		}
	}

	if len(failedCheckups) > 0 {
		red("\nSome checkups failed:")

		for _, c := range failedCheckups {
			fail(fmt.Sprintf("\t%s\n", c.Name()))
		}
		return
	}

	green("\nAll checkups passed! Your Kolide launcher is healthy.")
}

// run logs the results of a checkup being run
func runCheckup(c diag.CheckupRunner) error {
	cyan("\nRunning checkup: ")
	header("%s\n", c.Name())

	var informationalOutput strings.Builder
	result, err := c.Run(&informationalOutput)

	// Capture any output regardless of success
	scanner := bufio.NewScanner(strings.NewReader(informationalOutput.String()))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		fmt.Fprintln(doctorWriter, scanner.Text())
	}

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
