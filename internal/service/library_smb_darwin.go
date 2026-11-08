package service

import (
	"strings"

	"golang.org/x/sys/unix"
)

func platformSMBPath(path string) bool {
	var mounts [256]unix.Statfs_t
	n, err := unix.Getfsstat(mounts[:], unix.MNT_NOWAIT)
	if err != nil {
		return false
	}
	best := ""
	smb := false
	if n > len(mounts) {
		n = len(mounts)
	}
	for _, m := range mounts[:n] {
		mount := strings.TrimRight(string(m.Mntonname[:]), "\x00")
		if (path == mount || strings.HasPrefix(path, strings.TrimRight(mount, "/")+"/")) && len(mount) > len(best) {
			best = mount
			smb = strings.TrimRight(string(m.Fstypename[:]), "\x00") == "smbfs"
		}
	}
	return smb
}
