//go:build linux
// +build linux

package trash

import (
	"bufio"
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

	// Read /proc/mounts to get all mount points
	mounts, err := getMountPoints()
	if err != nil {
		return "", err
	}

	// Find the longest matching mount point
	var bestMount string
	for _, mount := range mounts {
		if strings.HasPrefix(absPath, mount) && len(mount) > len(bestMount) {
			bestMount = mount
		}
	}

	if bestMount == "" {
		return "/", nil
	}

	return bestMount, nil
}

func getMountPoints() ([]string, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/mounts: %w", err)
	}
	defer file.Close()

	var mounts []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			mountPoint := fields[1]
			// Unescape special characters in mount points
			mountPoint = unescapeMountPoint(mountPoint)
			mounts = append(mounts, mountPoint)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read /proc/mounts: %w", err)
	}

	return mounts, nil
}

func unescapeMountPoint(s string) string {
	// /proc/mounts escapes special characters as octal sequences
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			// Check if this is an octal escape
			if isOctal(s[i+1]) && isOctal(s[i+2]) && isOctal(s[i+3]) {
				val := (s[i+1]-'0')*64 + (s[i+2]-'0')*8 + (s[i+3] - '0')
				result = append(result, byte(val))
				i += 3
				continue
			}
		}
		result = append(result, s[i])
	}
	return string(result)
}

func isOctal(c byte) bool {
	return c >= '0' && c <= '7'
}
