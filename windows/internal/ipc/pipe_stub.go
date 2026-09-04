//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// ListenPipe on non-Windows uses a Unix domain socket under the data dir for local tests.
func ListenPipe(name string) (net.Listener, error) {
	if name == "" {
		name = filepath.Join(os.TempDir(), "blockads.sock")
	}
	_ = os.Remove(name)
	ln, err := net.Listen("unix", name)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", name, err)
	}
	return ln, nil
}
