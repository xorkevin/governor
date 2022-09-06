package governor

import (
	"runtime/debug"
	"strings"
	"time"
)

type (
	VCSBuildInfo struct {
		GoVersion   string
		ModVersion  string
		VCS         string
		VCSRevision string
		VCSTime     time.Time
		VCSModified bool
	}
)

// ReadVCSBuildInfo reads vcs build info from [runtime/debug.ReadBuildInfo]
func ReadVCSBuildInfo() VCSBuildInfo {
	v := VCSBuildInfo{
		GoVersion:   "",
		ModVersion:  "(devel)",
		VCS:         "",
		VCSRevision: "",
		VCSTime:     time.Unix(0, 0),
		VCSModified: true,
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		v.GoVersion = info.GoVersion
		v.ModVersion = info.Main.Version
		for _, i := range info.Settings {
			switch i.Key {
			case "vcs":
				v.VCS = i.Value
			case "vcs.revision":
				v.VCSRevision = i.Value
			case "vcs.time":
				t, err := time.Parse(time.RFC3339, i.Value)
				if err == nil {
					v.VCSTime = t
				}
			case "vcs.modified":
				v.VCSModified = i.Value != "false"
			}
		}
	}
	return v
}

// VCSStr formats vcs info
func (v VCSBuildInfo) VCSStr() string {
	b := strings.Builder{}
	hasVCS := false
	if v.VCS != "" {
		hasVCS = true
		b.WriteString(v.VCS)
	}
	if v.VCSRevision != "" {
		if hasVCS {
			b.WriteString("-")
		}
		hasVCS = true
		b.WriteString(v.VCSRevision)
	}
	if v.VCSModified {
		if hasVCS {
			b.WriteString("-")
		}
		hasVCS = true
		b.WriteString("dev")
	}
	return b.String()
}
