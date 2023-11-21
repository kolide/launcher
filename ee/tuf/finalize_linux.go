//go:build linux
// +build linux

package tuf

import (
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/pkg/allowedcmd"
)

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

	cmd := allowedcmd.Patchelf(ctx, "--set-interpreter", interpreterLocation, executableLocation)
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

	// interpData should look something like "/lib64/ld-linux-x86-64.so.2"
	return filepath.Base(string(interpData)), nil
}

func findInterpreterInNixStore(interpreter string) (string, error) {
	storeLocationPattern := filepath.Join("/nix/store/*glibc*/lib", interpreter)

	matches, err := filepath.Glob(storeLocationPattern)
	if err != nil {
		return "", fmt.Errorf("globbing for interpreter %s at %s: %w", interpreter, storeLocationPattern, err)
	}

	return matches[0], nil
}
