//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// attachConsole ensures that subsequent output from the process will be
// printed to the user's terminal.
func attachConsole() error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attachConsoleProc := kernel32.NewProc("AttachConsole")

	// Call AttachConsole, using the console of the parent of the current process
	// See: https://learn.microsoft.com/en-us/windows/console/attachconsole
	r1, _, err := attachConsoleProc.Call(^uintptr(0))
	if r1 == 0 {
		return fmt.Errorf("could not call AttachConsole: %w", err)
	}

	// Set stdout for newly attached console
	stdout, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return fmt.Errorf("getting stdout handle: %w", err)
	}
	os.Stdout = os.NewFile(uintptr(stdout), "stdout")

	// Set stderr for newly attached console
	stderr, err := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	if err != nil {
		return fmt.Errorf("getting stderr handle: %w", err)
	}
	os.Stderr = os.NewFile(uintptr(stderr), "stderr")

	// Print an empty line so that our first line of actual output doesn't occur on the same line
	// as the command prompt
	fmt.Println("")

	return nil
}

// detachConsole undos a previous call to attachConsole. It will leave the window
// appearing to hang, so it notifies the user to press enter in order to get
// their command prompt back.
func detachConsole() error {
	// Let the user know they have to press enter to get their prompt back
	fmt.Println("Press enter to return to your terminal")

	// Now, free the console
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	freeConsoleProc := kernel32.NewProc("FreeConsole")

	// See: https://learn.microsoft.com/en-us/windows/console/freeconsole
	r1, _, err := freeConsoleProc.Call()
	if r1 == 0 {
		return fmt.Errorf("could not call FreeConsole: %w", err)
	}

	return nil
}
