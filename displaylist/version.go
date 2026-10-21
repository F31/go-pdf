package displaylist

import "fmt"

type VersionInfo struct {
	Major int
	Minor int
}

// Version is frozen at 1.0 when G0 completes. Field additions increment Minor;
// breaking changes increment Major and require an ADR.
var Version = VersionInfo{Major: 1, Minor: 0}

func (v VersionInfo) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// Compatible reports whether a consumer built for major can consume this
// package. Minor versions are backward compatible.
func Compatible(major int) bool { return major == Version.Major }
