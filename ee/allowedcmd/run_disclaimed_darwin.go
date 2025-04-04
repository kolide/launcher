//go:build darwin
// +build darwin

package allowedcmd

/*
#cgo CFLAGS: -mmacosx-version-min=10.14 -Wall -Werror
#cgo LDFLAGS: -framework Foundation -framework Security -framework System

#include <Security/Authorization.h>
#include <Security/AuthorizationTags.h>
#include <spawn.h>
#include <sys/types.h>
#include <sys/wait.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// see https://www.qt.io/blog/the-curious-case-of-the-responsible-process for a wonderful writeup
// of how this call works
int responsibility_spawnattrs_setdisclaim(posix_spawnattr_t attrs, int disclaim);

int spawn_disclaimed(const char *path, char *const argv[], char *const envp[], char **stdout, char **stderr) {
    posix_spawnattr_t attrs;
    int err = posix_spawnattr_init(&attrs);
    if (err != 0) {
        return err;
    }

    // Set the disclaim attribute
    err = responsibility_spawnattrs_setdisclaim(&attrs, 1);
    if (err != 0) {
        posix_spawnattr_destroy(&attrs);
        return err;
    }

    pid_t pid;
    err = posix_spawn(&pid, path, NULL, &attrs, argv, envp);
    posix_spawnattr_destroy(&attrs);

    if (err != 0) {
        return err;
    }

    int status;
    if (waitpid(pid, &status, 0) == -1) {
        return -1;
    }

   // WIFEXITED will return non-zero if the child process terminated normally
   return WIFEXITED(status) ? WEXITSTATUS(status) : -1;
}
*/
import "C"

import (
	"bytes"
	"unsafe"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func Run(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {

	// if _, err := os.Stdout.Write(cmdOutput); err != nil {
	// 	return fmt.Errorf("writing results: %w", err)
	// }

	command := C.CString(cmd)
	defer C.free(unsafe.Pointer(command))

	cargs := make([]*C.char, len(args)+2) // should be building up like []*C.char{command, arg1, arg2..., nil}
	cargs[0] = command
	for i, arg := range args {
		cargs[i+1] = C.CString(arg)
	}
	cargs[len(cargs)-1] = nil // should be terminated by null entry

	defer func() {
		for _, arg := range cargs {
			if arg != nil {
				C.free(unsafe.Pointer(arg))
			}
		}
	}()

	cenvs := make([]*C.char, len(envs)+1)
	for i, arg := range envs {
		cenvs[i] = C.CString(arg)
	}
	cenvs[len(cenvs)-1] = nil // should be terminated by null entry

	defer func() {
		for _, arg := range cenvs {
			if arg != nil {
				C.free(unsafe.Pointer(arg))
			}
		}
	}()

	var output, stderr *C.char

	C.spawn_disclaimed(command, &cargs[0], &cenvs[0], &output, &stderr)

	goOut := C.GoString(output)
	goErr := C.GoString(stderr)

	C.free(unsafe.Pointer(output))
	C.free(unsafe.Pointer(stderr))

	// combine them ourselves
	return bytes.NewBufferString(goOut + goErr), nil
}

func validatedCmd(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {

}
