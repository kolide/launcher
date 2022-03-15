package packaging

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/kolide/launcher/pkg/packagekit"
	"github.com/stretchr/testify/require"
)

func helperCommandContext(ctx context.Context, command string, args ...string) (cmd *exec.Cmd) {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd = exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestExecOut(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &PackageOptions{}
	p.execCC = helperCommandContext

	response, err := p.execOut(ctx, "echo", "one", "and", "two")
	require.NoError(t, err)
	require.Equal(t, "one and two", response)

	// This fails because we didn't mock the "not mocked" command
	failResponse, err := p.execOut(ctx, "not mocked", "echo", "one", "and", "two")
	require.Error(t, err)
	require.Equal(t, "", failResponse)

}

func TestInitStuff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initOptions := &packagekit.InitOptions{
		Name:        "test",
		Description: "A test init",
		Path:        "/a/path",
		Identifier:  "test",
		Flags:       []string{},
		Environment: map[string]string{},
	}

	for _, target := range testedTargets() {
		testPackageRoot, err := ioutil.TempDir("", fmt.Sprintf("test-packaging-root-%s", target.String()))
		require.NoError(t, err)

		testScriptDir, err := ioutil.TempDir("", fmt.Sprintf("test-packaging-script-%s", target.String()))
		require.NoError(t, err)

		p := &PackageOptions{
			target:      target,
			Identifier:  "test",
			initFile:    "/usr/bin/true",
			scriptRoot:  testScriptDir,
			packageRoot: testPackageRoot,
			initOptions: initOptions,
		}

		// TODO we're creating here, but we should check the output
		require.NoError(t, p.setupInit(ctx))
		require.NoError(t, p.setupPostinst(ctx))
		require.NoError(t, p.setupPrerm(ctx))
	}
}

// TestHelperProcess isn't a real test. It's used as a helper process
// for TestParameterRun. It's comes from both
// https://github.com/golang/go/blob/master/src/os/exec/exec_test.go#L724
// and https://npf.io/2015/06/testing-exec-command/
func TestHelperProcess(t *testing.T) {
	t.Parallel()

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
	case strings.HasSuffix(cmd, "launcher") && args[0] == "-version":
		fmt.Println(`launcher - version 0.5.6-19-g17c8589
  branch: 	master
  revision: 	17c8589f47858877bb8de3d8ab1bd095cf631a11
  build date: 	2018-11-09T15:31:10Z
  build user: 	seph
  go version: 	go1.11`)
	default:
		fmt.Fprintf(os.Stderr, "Can't mock, unknown command(%q) args(%q) -- Fix TestHelperProcess", cmd, args)
		os.Exit(2)
	}

}

func testedTargets() []Target {
	return []Target{
		{
			Platform: Darwin,
			Init:     LaunchD,
			Package:  Pkg,
		},
		{
			Platform: Darwin,
			Init:     NoInit,
			Package:  Pkg,
		},
		{
			Platform: Linux,
			Init:     Systemd,
			Package:  Rpm,
		},
		{
			Platform: Linux,
			Init:     Systemd,
			Package:  Deb,
		},
		{
			Platform: Linux,
			Init:     NoInit,
			Package:  Deb,
		},
	}
}
