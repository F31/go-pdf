//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package pdf

import (
	"os"
	"syscall"
)

func mapReadOnly(f *os.File, size int64) ([]byte, func(), error) {
	if size <= 0 {
		return []byte{}, func() {}, nil
	}
	if size > int64(int(^uint(0)>>1)) {
		return nil, nil, os.ErrInvalid
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		data, rerr := readAll(f, size)
		if rerr != nil {
			return nil, nil, rerr
		}
		return data, func() {}, nil
	}
	return data, func() { _ = syscall.Munmap(data) }, nil
}
