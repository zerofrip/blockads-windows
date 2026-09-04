package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"
)

// Lock holds exclusive controller ownership for the process lifetime.
type Lock struct {
	release func() error
}

func (l *Lock) Release() {
	if l != nil && l.release != nil {
		_ = l.release()
		l.release = nil
	}
}

// Acquire attempts exclusive ownership.
// dataDir scopes the lock on non-Windows (and as a fallback path).
func Acquire(dataDir string) (*Lock, error) {
	return acquirePlatform(dataDir)
}

func lockFilePath(dataDir string) string {
	if dataDir == "" {
		dataDir = os.TempDir()
	}
	return filepath.Join(dataDir, "blockads-controller.lock")
}

func errInUse() error {
	return fmt.Errorf("another BlockAds controller is already running")
}

