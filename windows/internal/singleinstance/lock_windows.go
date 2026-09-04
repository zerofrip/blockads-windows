//go:build windows

package singleinstance

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// mutexNameForDataDir scopes the Global mutex so parallel tests with different
// TempDirs do not collide, while production PROGRAMDATA still gets one machine-
// wide controller lock per data directory.
func mutexNameForDataDir(dataDir string) string {
	clean := strings.ToLower(filepath.Clean(dataDir))
	sum := sha256.Sum256([]byte(clean))
	return "Global\\BlockAdsController_" + hex.EncodeToString(sum[:8])
}

func acquirePlatform(dataDir string) (*Lock, error) {
	name, err := windows.UTF16PtrFromString(mutexNameForDataDir(dataDir))
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
