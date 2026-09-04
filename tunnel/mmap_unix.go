//go:build unix

package tunnel

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// mapReadOnly memory-maps size bytes of file for reading.
// The caller must keep file open for the lifetime of the mapping on all platforms.
func mapReadOnly(file *os.File, size int64) (*mappedFile, error) {
	if file == nil {
		return nil, fmt.Errorf("nil file")
	}
	if size <= 0 {
		return nil, fmt.Errorf("invalid map size %d", size)
	}
	if size != int64(int(size)) {
		return nil, fmt.Errorf("map size too large: %d", size)
	}

	data, err := unix.Mmap(int(file.Fd()), 0, int(size), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap failed: %w", err)
	}

	return &mappedFile{
		data: data,
		unmap: func(b []byte) error {
			return unix.Munmap(b)
		},
	}, nil
}
