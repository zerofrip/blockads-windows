package tunnel

// mappedFile is a read-only memory mapping of a file.
// Platform-specific mapping lives in mmap_unix.go / mmap_windows.go.
type mappedFile struct {
	data   []byte
	unmap  func([]byte) error
	handle uintptr // optional platform handle (e.g. Windows mapping object)
}

// Bytes returns the mapped memory. Nil if closed or invalid.
func (m *mappedFile) Bytes() []byte {
	if m == nil {
		return nil
	}
	return m.data
}

// Close releases the mapping. Safe to call multiple times.
func (m *mappedFile) Close() error {
	if m == nil || m.data == nil {
		return nil
	}
	data := m.data
	m.data = nil
	var err error
	if m.unmap != nil {
		err = m.unmap(data)
		m.unmap = nil
	}
	m.handle = 0
	return err
}
