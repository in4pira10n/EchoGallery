package service

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func platformSMBPath(path string) bool {
	// Read the mapped drive's provider locally. DRIVE_REMOTE alone would also
	// classify WebDAV/NFS drives as SMB and change their automatic scan behavior.
	drive := strings.TrimSuffix(filepath.VolumeName(path), ":")
	if len(drive) != 1 {
		return false
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Network\`+strings.ToUpper(drive), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	provider, _, err := key.GetIntegerValue("ProviderType")
	return err == nil && provider == 0x00020000 // WNNC_NET_LANMAN (SMB)
}
