package runwrapper

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"testing"

	"github.com/stretchr/testify/require"
)

func xTestExec(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name   string // test name, optional, for debugging errors
		arg0   string
		args   []string
		opts   []Option
		stdout string
		stderr string
		err    error
	}{
		{
			arg0: "true",
		},
		{
			arg0: "false",
			err:  errors.New(""),
		},
		{
			arg0: "sleep",
			args: []string{"4"},
		},
		{
			arg0: "sleep",
			args: []string{"20"},
			err:  errors.New(""),
		},
		//{
		//	arg0: "true",
		//	opts: []Option{RunAsUid("0")},
		//},
	}

	for _, tt := range tests {
		testInfo := fmt.Sprintf("arg0: %s\nargs: %+v", tt.arg0, tt.args)

		if tt.name != "" {
			testInfo = fmt.Sprintf("name: %s\n%s", tt.name, testInfo)
		}

		stdout, stderr, err := Exec(context.TODO(), tt.arg0, tt.args, tt.opts...)
		if tt.err == nil {
			require.NoError(t, err, testInfo)
		} else {
			require.Error(t, err, testInfo)
		}
		require.Equal(t, tt.stdout, stdout, testInfo)
		require.Equal(t, tt.stderr, stderr, testInfo)
	}
}

func xTestExecRunAs(t *testing.T) {
	t.Parallel()

	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}

	if currentUser.Uid != "0" {
		t.Skip("Not root, can't test runas. Instead", currentUser.Uid)
	}
}

func TestExecRunAsNonRoot(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name   string // test name, optional, for debugging errors
		opts   []Option
		stdout string
		stderr string
		err    error
	}{
		{
			name:   "different uid, should fail",
			opts:   []Option{RunAsUid("0")},
			stdout: "root",
			//err:    errors.New(""),
		},
		{
			name:   "Same uid, should shortcut",
			stdout: "seph",
			opts:   []Option{RunAsUid("501")},
			//err:    errors.New(""),
		},
	}

	for _, tt := range tests {
		testInfo := fmt.Sprintf("name: %s", tt.name)
		stdout, stderr, err := Exec(context.TODO(), "/usr/bin/whoami", []string{}, tt.opts...)
		if tt.err == nil {
			require.NoError(t, err, testInfo)
		} else {
			require.Error(t, err, testInfo)
		}
		require.Equal(t, tt.stdout, stdout, testInfo)
		require.Equal(t, tt.stderr, stderr, testInfo)
	}
}
