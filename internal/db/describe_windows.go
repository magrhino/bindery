//go:build windows

package db

import (
	"fmt"
	"os"
)

// describeDir is the Windows fallback: we skip uid/gid (no POSIX Stat_t) and
// return just the path and mode so the SQLite error still gets a useful hint.
func describeDir(path string, info os.FileInfo) string {
	return fmt.Sprintf("%s mode=%s — ensure this directory is writable by the user running bindery",
		path, info.Mode().Perm().String())
}
