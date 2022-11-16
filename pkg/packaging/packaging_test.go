package packaging

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kolide/launcher/pkg/packagekit"
	"github.com/stretchr/testify/assert"
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
		target := target

		t.Run(target.String(), func(t *testing.T) {
			t.Parallel()

			testPackageRoot := t.TempDir()
			testScriptDir := t.TempDir()

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
		})
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

func Test_getBinary(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.SkipNow()
	}

	// Set up cache directory with only a launcher binary in it
	tmpCacheDir := t.TempDir()
	binaryName := "launcher"
	version := "nightly"
	localBinaryDir := filepath.Join(tmpCacheDir, fmt.Sprintf("launcher-%s-%s", runtime.GOOS, version))
	err := os.Mkdir(localBinaryDir, 0755)
	assert.NoError(t, err, "could not make temp cache directory")
	cachedBinaryPath := filepath.Join(localBinaryDir, binaryName)
	f, err := os.Create(cachedBinaryPath)
	assert.NoError(t, err, "could not create binary")
	defer f.Close()

	// Set up output directory
	tmpPkgRoot := t.TempDir()
	binDir := "bin"
	err = os.Mkdir(filepath.Join(tmpPkgRoot, binDir), 0755)
	assert.NoError(t, err, "could not make temp output directory")

	p := &PackageOptions{
		packageRoot: tmpPkgRoot,
		binDir:      binDir,
	}

	// Verify we found the non-app bundle binary and copied it to the expected location
	err = p.getBinary(context.TODO(), binaryName, binaryName, cachedBinaryPath)
	assert.NoError(t, err, "expected to find binary but did not")

	_, err = os.Stat(filepath.Join(tmpPkgRoot, binDir, binaryName))
	assert.NoError(t, err, "did not find binary in output directory")

	// Test looking for app bundle, if we're on macOS
	if runtime.GOOS == "darwin" {
		// Set up app bundle directory structure
		appBundleLocation := filepath.Join(localBinaryDir, "Kolide.app")
		err := os.MkdirAll(filepath.Join(appBundleLocation, "Contents", "MacOS"), 0755)
		assert.NoError(t, err, "could not make temp app bundle directory")

		// Add binary to app bundle
		f, err := os.Create(filepath.Join(appBundleLocation, "Contents", "MacOS", binaryName))
		assert.NoError(t, err, "could not create app bundle binary")
		defer f.Close()

		// Verify we found the app bundle and copied over the entire directory to the expected location
		err = p.getBinary(context.TODO(), binaryName, binaryName, cachedBinaryPath)
		assert.NoError(t, err, "expected to find app bundle but did not")

		appBundleInfo, err := os.Stat(filepath.Join(tmpPkgRoot, binDir, "Kolide.app"))
		assert.NoError(t, err, "did not find app bundle in output directory")
		assert.True(t, appBundleInfo.IsDir(), "app bundle not copied over correctly")

		binaryInfo, err := os.Stat(filepath.Join(tmpPkgRoot, binDir, "Kolide.app", "Contents", "MacOS", binaryName))
		assert.NoError(t, err, "did not find app bundle binary in output directory")
		assert.False(t, binaryInfo.IsDir(), "app bundle binary not copied over correctly")
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
