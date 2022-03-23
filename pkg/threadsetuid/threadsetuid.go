package threadsetuid

import(
	"time"
	"syscall"
	"fmt"
	"runtime"
)


func Runas(fn func()  error, timeout time.Duration, uid uint32, gid uint32)  error {
	// As we only support calling the function once, so we can
	// buffer a single size. Makes it a bit easier to
	// sequence starting the child, and handling the data
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

func pthread_setugid_np(uid uint32, gid uint32) (error) {
	// syscall.RawSyscall blocks, while Syscall does not. Since we
	// don't know better, we should use Syscall.
	_, _, e1 := syscall.Syscall(syscall.SYS_SETTID, uintptr(uid), uintptr(gid), 0)
	if e1 != 0 {
		return e1
	} 

	return nil
}

