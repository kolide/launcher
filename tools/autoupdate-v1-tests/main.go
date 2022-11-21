package main

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/kardianos/osext"
)

type thingy struct {
	binaryName string
	stagedFile string
}

type processNotes struct {
	Pid     int
	Path    string
	Size    int64
	ModTime time.Time
}

var ProcessNotes processNotes

func main() {

	binaryName, err := osext.Executable()
	if err != nil {
		panic(fmt.Errorf("osext.Executable: %w", err))
	}

	processStat, err := os.Stat(binaryName)
	if err != nil {
		panic(fmt.Errorf("os.Stat: %w", err))
	}

	ProcessNotes.Pid = os.Getpid()
	ProcessNotes.Path = binaryName
	ProcessNotes.Size = processStat.Size()
	ProcessNotes.ModTime = processStat.ModTime()

	if len(os.Args) < 2 {
		// Let's assume this should be windows services for now
		//panic("Need an argument")
		_ = runWindowsSvc([]string{})
	}

	var run func([]string) error

	switch os.Args[1] {
	case "run":
		run = runLoop
	case "svc":
		run = runWindowsSvc
	case "svc-fg":
		run = runWindowsSvcForeground
	case "install":
		run = runInstallService
	case "uninstall":
		run = runRemoveService
	default:
		panic("Unknown option")
	}

	err = run(os.Args[2:])
	if err != nil {
		panic(fmt.Errorf("Running subcommand %s: %w", os.Args[1], err))
	}

	fmt.Printf("Finished Main (pid %d)\n", ProcessNotes.Pid)

}

func (b *thingy) rename() error {
	tmpCurFile := fmt.Sprintf("%s-old", b.binaryName)

	fmt.Println("os.Rename cur to old")
	if err := os.Rename(b.binaryName, tmpCurFile); err != nil {
		return fmt.Errorf("os.Rename cur top old: %w", err)
	}

	fmt.Println("os.Rename stage to cur")
	if err := os.Rename(b.stagedFile, b.binaryName); err != nil {
		return fmt.Errorf("os.Rename staged to cur: %w", err)
	}

	fmt.Println("os.Chmod")
	if err := os.Chmod(b.binaryName, 0755); err != nil {
		return fmt.Errorf("os.Chmod: %w", err)
	}

	fmt.Println("syscall.Exec")
	if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
		return fmt.Errorf("syscall.Exec: %w", err)
	}

	return nil
}
