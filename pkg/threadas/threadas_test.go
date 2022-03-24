//go:build darwin
// +build darwin

package threadas

import (
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const timeout = 100 * time.Millisecond

func TestThreadAsNotRoot(t *testing.T) {
	t.Parallel()

	if syscall.Getuid() == 0 {
		t.Skip("Skipping -- test requires not-root")
	}

	uids := getUidData()
	fn := uids.GenerateTestFunc(t, "expected failure", assert.Equal)
	require.Error(t, ThreadAs(fn, timeout, uint32(uids.Uid), uint32(uids.Gid)), "Fails when run as non-root")
}

func TestThreadAs(t *testing.T) {
	t.Parallel()

	if syscall.Getuid() != 0 {
		t.Skip("Skipping -- test requires root")
	}

	targetUid := 501
	targetGid := 20

	targetUids := uidData{Uid: targetUid, Euid: targetUid, Gid: targetGid, Egid: targetGid}
	fnTargetUidsEqual := targetUids.GenerateTestFunc(t, "matches target user", assert.Equal)
	fnTargetUidsNotEqual := targetUids.GenerateTestFunc(t, "does not matches target user", assert.NotEqual)

	myUids := getUidData()
	fnMyUidsEqual := myUids.GenerateTestFunc(t, "matches my user", assert.Equal)
	fnMyUidsNotEqual := myUids.GenerateTestFunc(t, "does not matches my user", assert.NotEqual)

	// Be somewhat thoughtful in how parallel works here -- using
	// t.Parallel will spawn a new thread, which potentially
	// undermines some of what we're testing
	for i := 1; i < 100; i++ {
		t.Run("", func(t *testing.T) {
			t.Run("baseline", func(t *testing.T) {
				require.NoError(t, fnMyUidsEqual(), "no thread, matches my uid")
				require.NoError(t, fnTargetUidsNotEqual(), "no thread, does not match target")
			})

			t.Run("change to target uid", func(t *testing.T) {
				require.NoError(t, ThreadAs(fnTargetUidsEqual, timeout, uint32(targetUid), uint32(targetGid)), "match target")
				require.NoError(t, ThreadAs(fnMyUidsNotEqual, timeout, uint32(targetUid), uint32(targetGid)), "not match mine")
			})

			t.Run("change to my uid", func(t *testing.T) {
				require.NoError(t, ThreadAs(fnTargetUidsNotEqual, timeout, uint32(0), uint32(0)), "not match target")
				require.NoError(t, ThreadAs(fnMyUidsEqual, timeout, uint32(0), uint32(0)), "matches mine")
			})

			t.Run("baseline after setuids", func(t *testing.T) {
				require.NoError(t, fnMyUidsEqual(), "no thread, matches my uid")
				require.NoError(t, fnTargetUidsNotEqual(), "no thread, does not match target")
			})
		})
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()

	if syscall.Getuid() != 0 {
		t.Skip("Skipping -- test requires root")
	}

	fn := func() error {
		time.Sleep(10 * time.Second)
		return nil
	}

	require.Error(t, ThreadAs(fn, timeout, uint32(syscall.Getuid()), uint32(syscall.Getgid())))
}

func TestGoroutineLeaks(t *testing.T) { // nolint:paralleltest
	// Don't parallize this -- it's using the global count of
	// goroutines, which is going to vary based on what other
	// tests are running.

	if syscall.Getuid() != 0 {
		t.Skip("Skipping -- test requires root")
	}

	startCount := runtime.NumGoroutine()

	fn := func() error {
		c := runtime.NumGoroutine()
		require.Equal(t, startCount+1, c, "go routines should be one more than starting")
		return nil
	}

	require.NoError(t, ThreadAs(fn, timeout, uint32(syscall.Getuid()), uint32(syscall.Getgid())))

	require.Equal(t, startCount, runtime.NumGoroutine(), "go routines should return to normal")

}

// TestTestUids tests that my test harness basically works
func TestTestUids(t *testing.T) {
	t.Parallel()

	uids := getUidData()
	uids.TestUids(t, "bare test", assert.Equal)

	fn := uids.GenerateTestFunc(t, "generated", assert.Equal)
	require.NoError(t, fn())
}

type uidData struct {
	Uid  int
	Euid int
	Gid  int
	Egid int
}

// GenerateTestFunc generates a function suitable for handing to ThreadAs
func (ud uidData) GenerateTestFunc(t *testing.T, name string, assertion assert.ComparisonAssertionFunc) func() error {
	return func() error {
		ud.TestUids(t, name, assertion)
		return nil
	}
}

func (ud uidData) TestUids(t *testing.T, name string, assertion assert.ComparisonAssertionFunc) {
	// Don't use t.Run here, it will spawn a new thread and thus reset any kind of setuid
	actual := getUidData()

	assertion(t, ud.Uid, actual.Uid, "uid")
	assertion(t, ud.Euid, actual.Euid, "euid")
	assertion(t, ud.Gid, actual.Gid, "gid")
	assertion(t, ud.Egid, actual.Egid, "egid")
}

func getUidData() uidData {
	return uidData{
		Uid:  syscall.Getuid(),
		Euid: syscall.Geteuid(),
		Gid:  syscall.Getgid(),
		Egid: syscall.Getegid(),
	}
}
