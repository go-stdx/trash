//go:build windows
// +build windows

package trash

import (
	"errors"
	"strings"
)

func isCrossDeviceError(err error) bool {
	// On Windows, check for specific error messages that indicate cross-device moves
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "The system cannot move the file to a different disk drive") ||
		strings.Contains(errStr, "incorrect function")
}
