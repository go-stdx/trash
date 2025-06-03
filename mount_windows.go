//go:build windows
// +build windows

package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func getMountPoint(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// On Windows, the mount point is the drive letter
	if len(absPath) >= 2 && absPath[1] == ':' {
		return absPath[:2] + string(filepath.Separator), nil
	}

	return "", fmt.Errorf("unable to determine drive for path: %s", absPath)
}

func getMountPoints() ([]string, error) {
	// On Windows, we'll just return common drive letters
	// In a production system, you'd want to use Windows API to get actual drives
	var drives []string
	for c := 'A'; c <= 'Z'; c++ {
		drive := fmt.Sprintf("%c:\\", c)
		// Check if drive exists by trying to stat it
		if _, err := os.Stat(drive); err == nil {
			drives = append(drives, drive)
		}
	}
	return drives, nil
}
