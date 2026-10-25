package displaylist

import "fmt"

type VersionInfo struct {
	Major int
	Minor int
}

// Version tracks the public display-list contract. Field additions increment
// Minor; breaking changes increment Major and require an ADR.
var Version = VersionInfo{Major: 2, Minor: 0}

func (v VersionInfo) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// Compatible reports whether a consumer built for major can consume this
// package. Minor versions are backward compatible.
func Compatible(major int) bool { return major == Version.Major }
