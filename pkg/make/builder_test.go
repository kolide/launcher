package make

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/stretchr/testify/require"
)

func helperCommandContext(ctx context.Context, command string, args ...string) (cmd *exec.Cmd) {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd = exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}

	// Do we have an ENV key? (type assert)
	if ctxEnv, ok := ctx.Value("ENV").([]string); ok {
		cmd.Env = append(cmd.Env, ctxEnv...)
	}

	return cmd
}

func TestNamingHelpers(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		b            Builder
		extensionOut string
		binaryOut    string
	}{
		{
			b:            Builder{os: "linux"},
			extensionOut: "build/linux/test.ext",
			binaryOut:    "build/linux/test",
		},
		{
			b:            Builder{os: "windows"},
			extensionOut: "build/windows/test.exe",
			binaryOut:    "build/windows/test.exe",
		},
		{
			b:            Builder{os: "darwin"},
			extensionOut: "build/darwin/test.ext",
			binaryOut:    "build/darwin/test",
		},
	}

	for _, tt := range tests {
		require.Equal(t, tt.binaryOut, tt.b.PlatformBinaryName("test"))
		require.Equal(t, tt.extensionOut, tt.b.PlatformExtensionName("test"))
	}
}

func TestGoVersionCompatible(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		ver    string
		passes bool
	}{
		{
			ver:    "1.10",
			passes: false,
		},
		{
			ver:    "1.10.1",
			passes: false,
		},
		{
			ver:    "1.11",
			passes: true,
		},
		{
			ver:    "1.12",
			passes: true,
		},
	}

	for _, tt := range tests {
		semVer, err := semver.NewVersion(tt.ver)
		require.NoError(t, err)

		b := Builder{goVer: semVer}
		err = b.goVersionCompatible()
		if tt.passes {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestDepsGo(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	semVer, err := semver.NewVersion("1.11")
	require.NoError(t, err)

	b := Builder{goVer: semVer}
	b.execCC = helperCommandContext

	err = b.DepsGo(ctx)
	require.NoError(t, err)
}

func TestExecOut(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := &Builder{}
	b.execCC = helperCommandContext

	response, err := b.execOut(ctx, "echo", "one", "and", "two")
	require.NoError(t, err)
	require.Equal(t, "one and two", response)

	// This fails because we didn't mock the "not mocked" command
	failResponse, err := b.execOut(ctx, "not mocked", "echo", "one", "and", "two")
	require.Error(t, err)
	require.Equal(t, "", failResponse)

}

func TestGetVersion(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		in  string
		out string
		err bool
	}{
		{
			in:  "",
			err: true,
		},
		{
			in:  "1",
			err: true,
		},
		{
			in:  "0.1",
			out: "0.1.0",
		},
		{
			in:  "0.1-sha123",
			out: "0.1.0-sha123",
		},
		{
			in:  "0.10.3",
			out: "0.10.3",
		},
		{
			in:  "v0.11.0",
			out: "0.11.0",
		},
		{
			in:  "v0.11.0-1-gd6d5a56-dirty",
			out: "0.11.0-1-gd6d5a56-dirty",
		},
		{
			in:  "1.2.3-4",
			out: "1.2.3-4",
		},
		{
			in:  "0.1.2.3",
			err: true,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := &Builder{}
	b.execCC = helperCommandContext

	for _, tt := range tests {
		ctx = context.WithValue(ctx, "ENV", []string{fmt.Sprintf("FAKE_GIT_DESCRIBE=%s", tt.in)})
		os.Setenv("FAKE_GIT_DESCRIBE", tt.in)
		ver, err := b.getVersion(ctx)
		if tt.err == true {
			require.Error(t, err, tt.in)
			continue
		}

		require.NoError(t, err, tt.in)
		require.Equal(t, tt.out, ver, tt.in)
	}

}

// TestHelperProcess isn't a real test. It's used as a helper process
// for TestParameterRun. It's comes from both
// https://github.com/golang/go/blob/master/src/os/exec/exec_test.go#L724
// and https://npf.io/2015/06/testing-exec-command/
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	if os.Getenv("GO_WANT_HELPER_PROCESS_FORCE_ERROR") == "1" {
		os.Exit(1)
	}

	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	cmd, args := args[0], args[1:]
	switch {
	case cmd == "echo":
		iargs := []interface{}{}
		for _, s := range args {
			iargs = append(iargs, s)
		}
		fmt.Println(iargs...)
	case cmd == "exit":
		n, _ := strconv.Atoi(args[0])
		os.Exit(n)
	case cmd == "go" && args[0] == "mod" && args[1] == "download":
		return
	case cmd == "git" && args[0] == "describe":
		fmt.Println(os.Getenv("FAKE_GIT_DESCRIBE"))
		return
	default:
		fmt.Fprintf(os.Stderr, "Can't mock, unknown command(%q) args(%q) -- Fix TestHelperProcess", cmd, args)
		os.Exit(2)
	}

}
