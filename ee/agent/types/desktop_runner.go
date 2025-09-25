package types

import "context"

// DesktopRunner interface provides access to desktop process management functionality
type DesktopRunner interface {
	// RequestProfile requests a profile of the specified type from all healthy desktop processes
	// Returns an array of file paths where the profiles were saved
	RequestProfile(ctx context.Context, profileType string) ([]string, error)
	// SetDesktopRunner is used by the knapsack to store the desktop runner reference
	SetDesktopRunner(runner DesktopRunner)
}
