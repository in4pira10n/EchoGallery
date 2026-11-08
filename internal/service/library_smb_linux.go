package service

import (
	"os"
	"strings"
)

func platformSMBPath(path string) bool {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	best := ""
	smb := false
	decode := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for _, line := range strings.Split(string(data), "\n") {
		halves := strings.SplitN(line, " - ", 2)
		if len(halves) != 2 {
			continue
		}
		fields := strings.Fields(halves[0])
		fs := strings.Fields(halves[1])
		if len(fields) < 5 || len(fs) == 0 {
			continue
		}
		mount := decode.Replace(fields[4])
		if (path == mount || strings.HasPrefix(path, strings.TrimRight(mount, "/")+"/")) && len(mount) > len(best) {
			best = mount
			smb = fs[0] == "cifs" || fs[0] == "smb3"
		}
	}
	return smb
}
