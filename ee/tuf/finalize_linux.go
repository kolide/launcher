//go:build linux
// +build linux

package tuf

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/pkg/allowedcmd"
)

// On NixOS, we have to set the interpreter for any non-NixOS executable we want to
// run. This means the binaries that our updater downloads.
// See: https://unix.stackexchange.com/a/522823
func patchExecutable(executableLocation string) error {
	if !allowedcmd.IsNixOS() {
		return nil
	}

	interpreter, err := getInterpreter(executableLocation)
	if err != nil {
		return fmt.Errorf("getting interpreter for %s: %w", executableLocation, err)
	}
	interpreterLocation, err := findInterpreterInNixStore(interpreter)
	if err != nil {
		return fmt.Errorf("finding interpreter %s in nix store: %w", interpreter, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := allowedcmd.Patchelf(ctx, "--set-interpreter", interpreterLocation, executableLocation)
	if err != nil {
		return fmt.Errorf("creating patchelf command: %w", err)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running patchelf: output `%s`, error `%w`", string(out), err)
	}

	return nil
}

func getInterpreter(executableLocation string) (string, error) {
	f, err := elf.Open(executableLocation)
	if err != nil {
		return "", fmt.Errorf("opening ELF file: %w", err)
	}
	defer f.Close()

	interpSection := f.Section(".interp")
	if interpSection == nil {
		return "", errors.New("no .interp section")
	}

	interpData, err := interpSection.Data()
	if err != nil {
		return "", fmt.Errorf("reading .interp section: %w", err)
	}

<<<<<<< HEAD
	// interpData should look something like "/lib64/ld-linux-x86-64.so.2"
	return filepath.Base(string(interpData)), nil
=======
	trimmedInterpData := bytes.TrimRight(interpData, "\x00")

	// interpData should look something like "/lib64/ld-linux-x86-64.so.2" -- grab just the filename
	return filepath.Base(string(trimmedInterpData)), nil
>>>>>>> 2741611e2760c9376e13a42d3ca8613bfe5253fb
}

func findInterpreterInNixStore(interpreter string) (string, error) {
	storeLocationPattern := filepath.Join("/nix/store/*glibc*/lib", interpreter)

	matches, err := filepath.Glob(storeLocationPattern)
	if err != nil {
		return "", fmt.Errorf("globbing for interpreter %s at %s: %w", interpreter, storeLocationPattern, err)
	}

	return matches[0], nil
}
