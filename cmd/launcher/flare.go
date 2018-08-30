package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"

	"github.com/kolide/kit/ulid"
)

func runFlare(args []string) error {
	flagset := flag.NewFlagSet("launcher flare", flag.ExitOnError)
	var ()
	flagset.Usage = commandUsage(flagset, "launcher flare")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	b := new(bytes.Buffer)
	id := ulid.New()

	output(b, stdout, "Starting Launcher Diagnostics\n")
	output(b, stdout, "\tID: %s\n", id)
	user, err := user.Current()
	if err != nil {
		fatal(b, err)
	}
	output(b, stdout, "\tCurrentUser: %s uid: %s\n", user.Username, user.Uid)

	return nil
}

type outputDestination int

const (
	fileOnly outputDestination = iota
	stdout
)

func fatal(w io.Writer, err error) {
	output(w, stdout, "error: %s\n", err)
	os.Exit(1)
}

func output(w io.Writer, printTo outputDestination, f string, a ...interface{}) error {
	if printTo == stdout {
		fmt.Printf(f, a...)
	}

	_, err := fmt.Fprintf(w, f, a...)
	return err
}
