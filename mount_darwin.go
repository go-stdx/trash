//go:build darwin
// +build darwin

package trash

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func getMountPoint(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// Use df command to get mount point
	cmd := exec.Command("df", absPath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run df: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("unexpected df output")
	}

	// Parse the second line
	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return "", fmt.Errorf("unexpected df output format")
	}

	// The last field is the mount point
	return fields[len(fields)-1], nil
}

func getMountPoints() ([]string, error) {
	cmd := exec.Command("mount")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run mount: %w", err)
	}

	var mounts []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		// Format: /dev/disk1s1 on / (apfs, local, read-only, system)
		line := scanner.Text()
		parts := strings.Split(line, " on ")
		if len(parts) == 2 {
			mountPoint := strings.Fields(parts[1])[0]
			mounts = append(mounts, mountPoint)
		}
	}

	return mounts, nil
}
