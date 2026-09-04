//go:build !windows

package singleinstance

import (
	"os"
	"syscall"
)

func acquirePlatform(dataDir string) (*Lock, error) {
	path := lockFilePath(dataDir)
	_ = os.MkdirAll(dataDir, 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, errInUse()
	}
	return &Lock{release: func() error {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return f.Close()
	}}, nil
}
