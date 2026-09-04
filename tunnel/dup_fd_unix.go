//go:build unix

package tunnel

import "syscall"

func dupFileDescriptor(fd int) (int, error) {
	return syscall.Dup(fd)
}
