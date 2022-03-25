//go:build darwin
// +build darwin

// Package threadas uses some thread trickery to facilitate
// dropping permissions on a single thread. This allows us to call
// underlying darwin APIs which change behavior based on the userid.
//
// Note that this changes permission on a single thread. It cannot
// change permission on something like osquery (communication is over
// a socket), and may not do the right thing if you bring in an
// exec. This also means that if you spawn a new thread, you'll get
// the parent process permissions.
//
// This is based on ideas from:
//   * https://github.com/golang/go/issues/14592
//   * https://wiki.freebsd.org/Per-Thread%20Credentials
//   * https://pkg.go.dev/runtime#LockOSThread
//
// There's future work in supporting other platforms. Linux may have
// `syscall(SYS_setresuid, ...)` for this.
package threadas

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
)

const (
	// KAUTH_UID_NONE and KAUTH_GID_NONE are special values. When
	// pthread_setugid_np is called with them, it will revert to
	// the process credentials.
	KAUTH_UID_NONE = ^uint32(0) - 100
	KAUTH_GID_NONE = ^uint32(0) - 100
)

type NoPermissionsError struct {
	Errno syscall.Errno
}

func (e *NoPermissionsError) Error() string {
	return fmt.Sprintf("error changing permission: %v", e.Errno)
}

func (e *NoPermissionsError) Is(target error) bool {
	_, ok := target.(*NoPermissionsError)
	return ok
}

func (e *NoPermissionsError) Unwrap() error {
	return e.Errno
}

type TimeoutError struct {
	t time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("timeout after %s", e.t)
}

func (e *TimeoutError) Is(target error) bool {
	_, ok := target.(*TimeoutError)
	return ok
}

// ThreadAs will run a function, in a "thread", after using setuid to
// change permissions on that thread. It uses `LockOSThread` so the
// thread terminates after with the function, this is to clean up from
// the permissions change.
func ThreadAs(fn func() error, timeout time.Duration, uid uint32, gid uint32) error {
	// As we only support calling the function once, so we can
	// buffer a single size. Makes it a bit easier to
	// sequence starting the child and our listener.
	errChan := make(chan error, 1)

	go func() {
		// Calling LockOSThread, without a subsequent Unlock,
		// will cause the thread to terminate when the
		// goroutine does. This seems simpler than resetting
		// the thread permissions.
		//
		// An alternate implementation, for darwin, would be
		// to use `pthread_setugid_np(KAUTH_UID_NONE,
		// KAUTH_GID_NONE)` to reset permissions
		// afterwards. See the manpage, or
		// https://github.com/rfjakob/gocryptfs/blob/master/internal/syscallcompat/sys_darwin.go
		// for an example.
		runtime.LockOSThread()

		if err := pthread_setugid_np(uid, gid); err != nil {
			errChan <- err
			return
		}

		errChan <- fn()
	}()

	select {
	case err := <-errChan:
		return err
	case <-time.After(timeout):
		return &TimeoutError{t: timeout}
	}
}

// pthread_setugid_np calls the darwin syscall `pthread_setugid_np`
// which sets the per-thread userid and groupid. We use this, and not
// syscall.Setuid, because the latter will impact _all_ threads.
//
// This is not suitable for security isolation. Something in thread
// can return to parent permissions be either calling with
// KAUTH_UID_NONE, or simply spawning a new thread.
func pthread_setugid_np(uid uint32, gid uint32) error {
	// syscall.RawSyscall blocks, while Syscall does not. Since we
	// don't think this blocks, we can use the _slightly_ more
	// performant RawSyscall
	if _, _, errno := syscall.RawSyscall(syscall.SYS_SETTID, uintptr(uid), uintptr(gid), 0); errno != 0 {
		return &NoPermissionsError{Errno: errno}
	}

	return nil
}
