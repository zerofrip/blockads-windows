//go:build !windows

package netwatch

import "context"

func startPlatform(ctx context.Context, h Handler) error {
	// No-op on non-Windows; controller still supports ReevaluateAdapters for tests.
	<-ctx.Done()
	return nil
}
