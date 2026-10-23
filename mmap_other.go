//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !solaris

package pdf

import "os"

func mapReadOnly(f *os.File, size int64) ([]byte, func(), error) {
	data, err := readAll(f, size)
	if err != nil {
		return nil, nil, err
	}
	return data, func() {}, nil
}
