//go:build windows

package tunnel

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mapReadOnly memory-maps size bytes of file for reading via CreateFileMapping/MapViewOfFile.
// The caller must keep file open for the lifetime of the mapping.
func mapReadOnly(file *os.File, size int64) (*mappedFile, error) {
	if file == nil {
		return nil, fmt.Errorf("nil file")
	}
	if size <= 0 {
		return nil, fmt.Errorf("invalid map size %d", size)
	}

	h, err := windows.CreateFileMapping(windows.Handle(file.Fd()), nil, windows.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateFileMapping: %w", err)
	}

	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("MapViewOfFile: %w", err)
	}

	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), int(size))
	base := addr
	mapping := h

	return &mappedFile{
		data:   data,
		handle: uintptr(mapping),
		unmap: func(_ []byte) error {
			var first error
			if err := windows.UnmapViewOfFile(base); err != nil {
				first = err
			}
			if err := windows.CloseHandle(mapping); err != nil && first == nil {
				first = err
			}
			return first
		},
	}, nil
}
