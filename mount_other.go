//go:build !linux && !darwin && !windows
// +build !linux,!darwin,!windows

package trash

import (
	"path/filepath"
)

// Fallback implementation for other systems
func getMountPoint(path string) (string, error) {
	// For unsupported systems, always use home trash
	return "/", nil
}

func getMountPoints() ([]string, error) {
	// Return only root for unsupported systems
	return []string{"/"}, nil
}
