package threadsetuid

import(
	"time"
	"syscall"
	"fmt"
	"runtime"
)


func Runas(fn func() ([]map[string]interface{}, error), timeout time.Duration, uid uint32, gid uint32) ([]map[string]interface{}, error) {
	// Only support getting one batch of data, so we can just
	// buffer a single size. Makes it a bit easier to sequence
	// starting the child, and handling the data
	dataChan := make(chan []map[string]interface{}, 1)
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

		data, err := fn()
		if err != nil {
			errChan <- err
			return
		}

		dataChan <- data
	}()

	select {
	case data := <- dataChan:
		return data, nil
	case err := <- errChan:
		return nil, err
	case <- time.After(timeout):
		return nil, fmt.Errorf("Timeout after %s", timeout)
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

