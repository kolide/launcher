//go:build darwin
// +build darwin

package rundisclaimed

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
// of how this works
int responsibility_spawnattrs_setdisclaim(posix_spawnattr_t attrs, int disclaim);

int spawn_disclaimed(const char *path, char *const argv[], char *const envp[], char **stdout, char **stderr) {
    posix_spawnattr_t attrs;
    int err = posix_spawnattr_init(&attrs);
    if (err != 0) {
        const char *errmsg = strerror(err);
        *stderr = malloc(strlen(errmsg) + 1);
        if (*stderr) {
            strcpy(*stderr, errmsg);
        }
        return err;
    }

    // Set the disclaim attribute
    err = responsibility_spawnattrs_setdisclaim(&attrs, 1);
    if (err != 0) {
        const char *errmsg = strerror(err);
        *stderr = malloc(strlen(errmsg) + 1);
        if (*stderr) {
            strcpy(*stderr, errmsg);
        }
        posix_spawnattr_destroy(&attrs);
        return err;
    }

    // set up the stdout/stderr redirects
    int pipefd_out[2];
    int pipefd_err[2];
    if (pipe(pipefd_out) || pipe(pipefd_err)) {
        const char *errmsg = "unable to create stdout/err pipes";
        *stderr = malloc(strlen(errmsg) + 1);
        if (*stderr) {
            strcpy(*stderr, errmsg);
        }
        posix_spawnattr_destroy(&attrs);
        return -1;
    }

    posix_spawn_file_actions_t file_actions;
    posix_spawn_file_actions_init(&file_actions);
    posix_spawn_file_actions_adddup2(&file_actions, pipefd_out[1], STDOUT_FILENO);
    posix_spawn_file_actions_adddup2(&file_actions, pipefd_err[1], STDERR_FILENO);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_out[0]);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_err[0]);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_out[1]);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_err[1]);

    pid_t pid;
    err = posix_spawn(&pid, path, &file_actions, &attrs, argv, envp);
    posix_spawnattr_destroy(&attrs);
    posix_spawn_file_actions_destroy(&file_actions);
    close(pipefd_out[1]);  // Close write ends of pipes in parent
    close(pipefd_err[1]);

    if (err != 0) {
        const char *errmsg = strerror(err);
        *stderr = malloc(strlen(errmsg) + 1);
        if (*stderr) {
            strcpy(*stderr, errmsg);
        }
        close(pipefd_out[0]); // close read ends
        close(pipefd_err[0]);
        return err;
    }

    char buffer[1024];
    size_t out_size = 0, err_size = 0;

    // Read stdout
    FILE *out_fp = fdopen(pipefd_out[0], "r");
    if (out_fp) {
        while (fgets(buffer, sizeof(buffer), out_fp)) {
            size_t len = strlen(buffer);
            *stdout = realloc(*stdout, out_size + len + 1);
            memcpy(*stdout + out_size, buffer, len + 1);
            out_size += len;
        }
        fclose(out_fp);
    }

    // Read stderr
    FILE *err_fp = fdopen(pipefd_err[0], "r");
    if (err_fp) {
        while (fgets(buffer, sizeof(buffer), err_fp)) {
            size_t len = strlen(buffer);
            *stderr = realloc(*stderr, err_size + len + 1);
            memcpy(*stderr + err_size, buffer, len + 1);
            err_size += len;
        }
        fclose(err_fp);
    }

    int status;
    if (waitpid(pid, &status, 0) == -1) {
        const char *errmsg = "waitpid encountered error";
        *stderr = malloc(strlen(errmsg) + 1);
        if (*stderr) {
            strcpy(*stderr, errmsg);
        }
        close(pipefd_out[0]);  // close read ends
        close(pipefd_err[0]);
        return -1;
    }

   close(pipefd_out[0]);  // close read ends
   close(pipefd_err[0]);

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

	// arg1 := C.CString("outdated")
	// defer C.free(unsafe.Pointer(arg1))
	// arg2 := C.CString("--json")
	// defer C.free(unsafe.Pointer(arg2))
	// argv := []*C.char{command, arg1, arg2, nil}

	// env1 := C.CString("HOMEBREW_NO_AUTO_UPDATE=1")
	// defer C.free(unsafe.Pointer(env1))
	// // TODO interpolate
	// env2 := C.CString("HOME=/Users/zackolson")
	// defer C.free(unsafe.Pointer(env2))

	// envp := []*C.char{env1, env2, nil}

	var output, stderr *C.char

	// fmt.Printf("ABOUT TO RUN COMMAND\n")

	C.spawn_disclaimed(command, &cargs[0], &cenvs[0], &output, &stderr)
	// fmt.Printf("RAN COMMAND - result %d\n", result)

	goOut := C.GoString(output)
	goErr := C.GoString(stderr)

	// res := C.GoInt(result)
	// fmt.Printf("GOT COMMAND STDOUT:\n%s\n", goOut)
	// fmt.Printf("GOT COMMAND STDERR:\n%s\n", goErr)

	C.free(unsafe.Pointer(output))
	C.free(unsafe.Pointer(stderr))

	// combine them ourselves
	return bytes.NewBufferString(goOut + goErr), nil
}

func validatedCmd(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
    
}
