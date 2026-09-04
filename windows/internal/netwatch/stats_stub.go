//go:build !windows

package netwatch

// Stats is a no-op on non-Windows.
func Stats() (callbacks, reevaluations uint64) { return 0, 0 }

// ResetStats is a no-op on non-Windows.
func ResetStats() {}
