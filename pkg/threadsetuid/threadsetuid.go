// Package threadsetuid uses some thread trickery to facilitate
// dropping permissions on a single thread. This allows us to call
// underlying darwin APIs which change behavior based on the userid.
//
// Note that this changes permission on a single thread. It cannot
// change permission on something like osquery (communication is over
// a socket), and may not do the right thing if you bring in an exec.
//
// This is based on ideas from:
//   * https://github.com/golang/go/issues/14592
//   * https://wiki.freebsd.org/Per-Thread%20Credentials
//   * https://pkg.go.dev/runtime#LockOSThread
//
// There's future work in supporting other platforms. Linux may have
// `syscall(SYS_setresuid, ...)` for this.
package threadsetuid

import(
	"time"
	"syscall"
	"fmt"
	"runtime"
)


// ThreadAs will run a function, in a "thread", after using setuid to
// change permissions on that thread. It uses `LockOSThread` so the
// thread terminates after with the function, this is to clean up from
// the permissions change.
func ThreadAs(fn func()  error, timeout time.Duration, uid uint32, gid uint32)  error {
	// As we only support calling the function once, so we can
	// buffer a single size. Makes it a bit easier to
	// sequence starting the child and our listener.
	errChan := make(chan error, 1)

	go func() {
		// Calling LockOSThread, without a subsequent Unlock,
		// will cause the thread to terminate when the
		// goroutine does. This seems simpler than resetting
		// the thread permissions.
		runtime.LockOSThread()

		if err := pthread_setugid_np(uid, gid); err != nil {
			errChan <- err
			return
		}

		errChan <- fn()
	}()

	select {
	case err := <- errChan:
		return err
	case <- time.After(timeout):
		return fmt.Errorf("Timeout after %s", timeout)
	}
}

// pthread_setugid_np calls the darwin syscall `pthread_setugid_np`
// which sets the per-thread userid and groupid. We use this, and not
// syscall.Setuid, because the latter will impact _all_ threads.
//
// There is some discussion in the man page that this is not suitable
// for security isolation. It's recommended you read and understand that.
func pthread_setugid_np(uid uint32, gid uint32) (error) {
	// syscall.RawSyscall blocks, while Syscall does not. Since we
	// don't think this blocks, we can use the _slightly_ more
	// performant RawSyscall
	_, _, errNo := syscall.RawSyscall(syscall.SYS_SETTID, uintptr(uid), uintptr(gid), 0)
	if errNo != 0 {
		return errNo
	} 

	return nil
}

