package main

import (
	"flag"
	"path/filepath"

	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

func runPostinst(args []string) error {
	flagset := flag.NewFlagSet("launcher postinstall", flag.ExitOnError)
	flagset.Usage = commandUsage(flagset, "launcher postinstall")

	var (
		flConfig = flagset.String("config", "", "config file to parse options from (optional)")
		flDebug  = flagset.Bool("debug", false, "Whether or not debug logging is enabled (default: false)")
	)

	if err := ff.Parse(flagset, args,
		ff.WithConfigFileFlag("config"),
		ff.WithIgnoreUndefined(true), // covers the config file _only_
		ff.WithConfigFileParser(ff.PlainParser),
	); err != nil {
		return err
	}

	if *flConfig == "" {
		return errors.New("Missing required `config` option")
	}

	targetPostinstFile := filepath.Join(filepath.Dir(*flConfig), "postinst-test.json")

	_ = flDebug
	return nil

}
