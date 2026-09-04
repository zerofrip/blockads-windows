//go:build windows

package singleinstance

import "golang.org/x/sys/windows"

func acquirePlatform(dataDir string) (*Lock, error) {
	_ = dataDir // Windows uses a machine-global named mutex
	name, err := windows.UTF16PtrFromString(MutexName)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return nil, errInUse()
	}
	if err != nil {
		return nil, err
	}
	return &Lock{release: func() error {
		_ = windows.ReleaseMutex(h)
		return windows.CloseHandle(h)
	}}, nil
}
