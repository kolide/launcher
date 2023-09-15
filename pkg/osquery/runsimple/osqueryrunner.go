package runsimple

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"runtime"
)

// osqueryRunner is a very simple osquery runtime manager. It's designed to start and stop osquery. It has
// no interactions with the osquery socket, it is purely a process manager.
type osqueryProcess struct {
	osquerydPath  string
	rootDirectory string
	stdout        io.Writer
	stderr        io.Writer
	stdin         io.Reader
}

type osqueryProcessOpt func(*osqueryProcess)

// WithRootDirectory is a functional option which allows the user to define the
// path where filesystem artifacts will be stored. This may include pidfiles,
// RocksDB database files, etc. If this is not defined, a temporary directory
// will be used.
func WithRootDirectory(path string) osqueryProcessOpt {
	return func(p *osqueryProcess) {
		p.rootDirectory = path
	}
}

// WithStdout is a functional option which allows the user to define where the
// stdout of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStdout(w io.Writer) osqueryProcessOpt {
	return func(p *osqueryProcess) {
		p.stdout = w
	}
}

// WithStderr is a functional option which allows the user to define where the
// stderr of the osquery process should be directed. By default, the output will
// be discarded. This should only be configured once.
func WithStderr(w io.Writer) osqueryProcessOpt {
	return func(p *osqueryProcess) {
		p.stderr = w
	}
}

func WithStdin(r io.Reader) osqueryProcessOpt {
	return func(p *osqueryProcess) {
		p.stdin = r
	}
}

func NewOsqueryProcess(osquerydPath string, opts ...osqueryProcessOpt) (*osqueryProcess, error) {
	p := &osqueryProcess{
		osquerydPath: osquerydPath,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p, nil
}

// RunSql will run a given blob by passing it in as stdin. Note that osquery is perticular. It needs the
// trailing semicolon, but it's real weird about line breaks, an may return as multiline json output. It
// is the responsibility of the caller to get the details right.
func (p osqueryProcess) RunSql(ctx context.Context, sql []byte) error {
	args := []string{
		"-S",
		"--config_path", "/dev/null",
		"--disable_events",
		"--disable_database",
		"--ephemeral",
		"--json",
	}

	if runtime.GOOS != "windows" {
		args = append(args, "--disable_audit")
	}

	p.stdin = bytes.NewReader(sql)

	cmd := exec.CommandContext(ctx, p.osquerydPath, args...)

	// It's okay for these to be nil, so we can just set them without checking.
	cmd.Stdin = p.stdin
	cmd.Stdout = p.stdout
	cmd.Stderr = p.stderr

	return cmd.Run()
}

func (p osqueryProcess) RunVersion(ctx context.Context) error {
	args := []string{
		"--version",
	}

	cmd := exec.CommandContext(ctx, p.osquerydPath, args...)

	// It's okay for these to be nil, so we can just set them without checking.
	cmd.Stdin = p.stdin
	cmd.Stdout = p.stdout
	cmd.Stderr = p.stderr

	return cmd.Run()
}

/*

func (p *osqueryProcess) Execute(ctx context.Context) error {
	// if this grows to replacing the larger osquery runtime, there are a lot of questions about how it will work.
	//  - cmd.Start does not block, Need to call cmd.Wait after it, So how do we manage the start, health, wait in the rest of the control flow?
	//  - Should osquery get it's own context? It makes some of the process management easier. But maybe not.

}


func (p *osqueryProcess) Stop() error {
	proc := p.cmd.Process
	if proc == nil {
		return errors.New("No process. Missing start?")
	}

	err := cmd.Process.Signal(interrupt)
	if err == nil {
		err = ctx.Err() // Report ctx.Err() as the reason we interrupted.
	} else if err.Error() == "os: process already finished" {
		errc <- nil
	}

}
*/
