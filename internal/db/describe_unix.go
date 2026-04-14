//go:build !windows

package db

import (
	"fmt"
	"os"
	"syscall"
)

// describeDir renders the parent directory's permissions + ownership in a
// form most Linux operators read fluently (matches `ls -ld`'s mode column
// plus a "uid:gid" tail). We skip username lookup to keep this cheap and
// dependency-free; numeric IDs are unambiguous anyway.
func describeDir(path string, info os.FileInfo) string {
	uid, gid := "?", "?"
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = fmt.Sprintf("%d", st.Uid)
		gid = fmt.Sprintf("%d", st.Gid)
	}
	return fmt.Sprintf("%s mode=%s owner=%s:%s — ensure this directory is writable by the UID running bindery (distroless nonroot is 65532)",
		path, info.Mode().Perm().String(), uid, gid)
}
