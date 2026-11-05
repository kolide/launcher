//go:build linux

package checkups

import (
	"fmt"
	"io/fs"
	"syscall"
	"time"
)

func platformPathInfo(_ string, fi fs.FileInfo, info map[string]any) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		info["sys_error"] = fmt.Sprintf("unexpected stat type %T", fi.Sys())
		return
	}
	info["uid"] = st.Uid
	info["gid"] = st.Gid
	info["ino"] = st.Ino
	info["nlink"] = st.Nlink
	info["ctime"] = time.Unix(st.Ctim.Unix()).UTC()
}
