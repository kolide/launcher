//go:build windows

package checkups

import (
	"fmt"
	"io/fs"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func platformPathInfo(path string, fi fs.FileInfo, info map[string]any) {
	attrs, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		info["sys_error"] = fmt.Sprintf("unexpected stat type %T", fi.Sys())
		return
	}
	info["attrs"] = fmt.Sprintf("%#x", attrs.FileAttributes)
	info["birth"] = time.Unix(0, attrs.CreationTime.Nanoseconds()).UTC()

	ctime, err := changeTime(path)
	if err != nil {
		info["ctime_error"] = err.Error()
		return
	}
	info["ctime"] = ctime.UTC()
}

// windows.FileBasicInfo returns a void* to this struct:
//
//	https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_basic_info
type fileBasicInfo struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
	_              uint32
}

func changeTime(path string) (time.Time, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return time.Time{}, err
	}

	h, err := windows.CreateFile(p,
		windows.FILE_READ_ATTRIBUTES,
		// do not block other processes from manipulating this file
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return time.Time{}, fmt.Errorf("opening: %w", err)
	}
	defer windows.CloseHandle(h)

	var fbi fileBasicInfo
	// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getfileinformationbyhandleex
	if err := windows.GetFileInformationByHandleEx(h, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&fbi)), uint32(unsafe.Sizeof(fbi))); err != nil {
		return time.Time{}, fmt.Errorf("querying basic info: %w", err)
	}

	// Windows' FileTime is a bit quirky. It's the count of how many 100 ns intervals since, you guessed it,
	// 1/1/1601!
	//
	// See: https://learn.microsoft.com/en-us/windows/win32/api/minwinbase/ns-minwinbase-filetime
	ft := windows.Filetime{LowDateTime: uint32(fbi.ChangeTime), HighDateTime: uint32(fbi.ChangeTime >> 32)}
	return time.Unix(0, ft.Nanoseconds()), nil
}
