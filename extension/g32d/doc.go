// Package g32d provides an optional pure-Go CCITT Group 3 2-D decoder.
//
// The bit reader, run tables and Group 4 mode machinery are adapted from
// golang.org/x/image/ccitt (BSD-3-Clause), with Group 3 EOL/tag and mixed
// 1-D/2-D row handling added here. It does not link CGO or external processes.
package g32d
