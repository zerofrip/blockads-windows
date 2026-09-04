package netwatch

import "context"

// Watcher notifies the controller when network interfaces change.
type Handler interface {
	ReevaluateAdapters(ctx context.Context) error
}

// Start begins watching. Implementation is platform-specific.
func Start(ctx context.Context, h Handler) error {
	return startPlatform(ctx, h)
}
