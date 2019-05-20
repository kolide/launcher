package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

type UninstallOptions struct {
	dryRun              bool
	userConfirmed       bool
	identifier          string
	identifierHumanName string
	execCC              func(context.Context, string, ...string) *exec.Cmd // Allows test overrides
}

func runUninstall(args []string) error {
	flagset := flag.NewFlagSet("launcher uninstall", flag.ExitOnError)
	flagset.Usage = func() { usage(flagset) }

	var (
		flDebug            = flagset.Bool("debug", false, "Whether or not debug logging is enabled")
		flDryRun           = flagset.Bool("dryrun", false, "Run in silmuate mode -- take no actions")
		flSkipConfirmation = flagset.Bool("skip-user-confirmation", false, "Skip user confirmation step")
	)

	ff.Parse(flagset, args,
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
		ff.WithEnvVarPrefix("KOLIDE_UNINSTALL"),
	)

	uo := &UninstallOptions{
		identifier:          "kolide",
		identifierHumanName: "Cloud",
		execCC:              exec.CommandContext,
	}

	if *flDryRun {
		uo.dryRun = true
	}

	if *flSkipConfirmation {
		uo.userConfirmed = true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = ctxlog.NewContext(ctx, logutil.NewServerLogger(*flDebug))

	return uo.Uninstall(ctx)
}

func (uo *UninstallOptions) promptUser(msg string) error {
	if uo.userConfirmed {
		return nil
	}

	fmt.Printf("\n\n\n%s\nAre you sure?\nEnter YES<return> to continue: ", msg)

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return errors.Wrap(err, "readstring")
	}

	text = strings.Replace(text, "\n", "", -1)

	if text != "YES" {
		fmt.Println("canceled")
		return errors.New("User canceled")
	}

	uo.userConfirmed = true
	return nil
}

func (uo *UninstallOptions) execOut(ctx context.Context, argv0 string, args ...string) (string, string, error) {
	logger := ctxlog.FromContext(ctx)

	cmd := uo.execCC(ctx, argv0, args...)

	level.Debug(logger).Log(
		"msg", "execing",
		"cmd", strings.Join(cmd.Args, " "),
	)

	if uo.dryRun {
		level.Debug(logger).Log("msg", "Skipped due to dryrun")
		return "", "", nil
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), errors.Wrapf(err, "run command %s %v, stderr=%s", argv0, args, stderr)
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
}

func (uo *UninstallOptions) removePath(ctx context.Context, removePath string) error {
	logger := ctxlog.FromContext(ctx)

	level.Debug(logger).Log("msg", "Removing path", "path", removePath)

	if _, err := os.Stat(removePath); err == nil {
		if uo.dryRun {
			level.Debug(logger).Log("msg", "Skipped due to dryrun")
			return nil
		}

		if err := os.RemoveAll(removePath); err != nil {
			return errors.Wrapf(err, "removing %s", removePath)
		}

	} else if os.IsNotExist(err) {
		level.Debug(logger).Log("msg", "Path already gone", "path", removePath)
	} else {
		return errors.Wrapf(err, "Unable to tell if %s exists", removePath)
	}

	return nil
}
