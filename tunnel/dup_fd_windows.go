//go:build windows

package tunnel

import "golang.org/x/sys/windows"

func dupFileDescriptor(fd int) (int, error) {
	p := windows.CurrentProcess()
	var h windows.Handle
	err := windows.DuplicateHandle(
		p,
		windows.Handle(fd),
		p,
		&h,
		0,
		false,
		windows.DUPLICATE_SAME_ACCESS,
	)
	if err != nil {
		return -1, err
	}
	return int(h), nil
}
