package main

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/kardianos/osext"
	"github.com/kolide/kit/fs"
	"github.com/pkg/errors"
)

type thingy struct {
	binaryName string
	stagedFile string
}

const serviceName = "sephexec"
const serviceDesc = "seph Update Testing"

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
		panic(errors.Wrap(err, "osext.Executable"))
	}

	processStat, err := os.Stat(binaryName)
	if err != nil {
		panic(errors.Wrap(err, "os.Stat"))
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
		panic(errors.Wrapf(err, "Running subcommand %s", os.Args[1]))
	}

	fmt.Printf("Finished Main (pid %d)\n", ProcessNotes.Pid)

}

func oldShit() {

	fmt.Printf("\n\nStarting a new thread. Pid: %d\n", os.Getpid())
	time.Sleep(1 * time.Second)

	binaryName, err := osext.Executable()
	if err != nil {
		panic(errors.Wrap(err, "osext.Executable"))
	}

	// Should this get a random append?
	stagedFile := fmt.Sprintf("%s-staged", binaryName)

	// To emulate a new version, just copy the current binary to the staged location
	fmt.Println("fs.CopyFile")
	if err := fs.CopyFile(binaryName, stagedFile); err != nil {
		panic(errors.Wrap(err, "fs.CopyFile"))
	}

	b := &thingy{
		binaryName: binaryName,
		stagedFile: stagedFile,
	}

	if err := b.rename(); err != nil {
		panic(err)
	}

	fmt.Printf("BAD BAD BAD old thread! (pid %d)\n", os.Getpid())

}

func (b *thingy) rename() error {
	tmpCurFile := fmt.Sprintf("%s-old", b.binaryName)

	fmt.Println("os.Rename cur to old")
	if err := os.Rename(b.binaryName, tmpCurFile); err != nil {
		return errors.Wrap(err, "os.Rename cur top old")
	}

	fmt.Println("os.Rename stage to cur")
	if err := os.Rename(b.stagedFile, b.binaryName); err != nil {
		return errors.Wrap(err, "os.Rename staged to cur")
	}

	fmt.Println("os.Chmod")
	if err := os.Chmod(b.binaryName, 0755); err != nil {
		return errors.Wrap(err, "os.Chmod")
	}

	fmt.Println("syscall.Exec")
	if err := syscall.Exec(os.Args[0], os.Args, os.Environ()); err != nil {
		return errors.Wrap(err, "syscall.Exec")
	}

	return nil
}
