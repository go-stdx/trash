package trash

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// findMountPoint walks up the directory tree until the device (stat.Dev) changes
// (or we reach "/"), identifying the filesystem's mount point. Only works on Linux.
func findMountPoint(p string) (string, error) {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("trash: failed to cast to *syscall.Stat_t for %s", absPath)
	}

	currentDev := stat.Dev
	dir := absPath

	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached "/", must be the mount point
			return dir, nil
		}
		pInfo, err := os.Stat(parent)
		if err != nil {
			return "", err
		}
		pStat, ok := pInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return "", fmt.Errorf("trash: failed to cast to *syscall.Stat_t for %s", parent)
		}
		if pStat.Dev != currentDev {
			// 'dir' was the mount point
			return dir, nil
		}
		dir = parent
	}
}

// xdgTrashDir tries to return the filesystem-specific trash folder:
//   <mountPoint>/.Trash-<UID>/files
// If that fails (e.g. read-only), we fall back to:
//   ~/.local/share/Trash/files
func xdgTrashDir(filename string) (string, error) {
	mountPoint, err := findMountPoint(filename)
	if err != nil {
		return "", err
	}

	uid := syscall.Getuid() // numeric user ID
	trashDir := filepath.Join(mountPoint, fmt.Sprintf(".Trash-%d", uid), "files")

	// Attempt to create the mount-specific trash dir if it doesn't exist
	if err := os.MkdirAll(trashDir, 0700); err == nil {
		// We can use the mount-specific trash
		return trashDir, nil
	}

	// Otherwise, fallback to ~/.local/share/Trash/files
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	fallback := filepath.Join(u.HomeDir, ".local", "share", "Trash", "files")
	if err := os.MkdirAll(fallback, 0700); err != nil {
		return "", err
	}
	return fallback, nil
}

// uniqueTrashName finds a name for the file in the trash directory
// that does not collide with an existing file. It keeps appending
// a numeric suffix until it finds something free.
func uniqueTrashName(trashDir, origName string) (string, error) {
	base := filepath.Base(origName)
	dest := filepath.Join(trashDir, base)

	// If there's no collision, we're good
	if _, err := os.Lstat(dest); os.IsNotExist(err) {
		return dest, nil
	}

	// Otherwise, append a numerical suffix: name.ext -> name.ext_1, etc.
	ext := filepath.Ext(base)
	nameOnly := strings.TrimSuffix(base, ext)
	i := 1
	for {
		newBase := fmt.Sprintf("%s_%d%s", nameOnly, i, ext)
		dest = filepath.Join(trashDir, newBase)
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			return dest, nil
		}
		i++
		// In worst case, this could loop many times, but extremely rare in practice.
	}
}

// createTrashInfoFile creates the ".trashinfo" metadata file in trashDir/../info
// corresponding to the file "trashedFilePath" (in the "files" dir).
// e.g. if "trashedFilePath" is "/.../Trash/files/foo.txt", the .trashinfo goes to
// "/.../Trash/info/foo.txt.trashinfo".
//
// The content is in the format:
//
//   [Trash Info]
//   Path=/absolute/original/path
//   DeletionDate=YYYY-MM-DDTHH:MM:SS
//
func createTrashInfoFile(trashedFilePath, originalPath string) error {
	trashDir := filepath.Dir(trashedFilePath)           // ".../Trash/files"
	parent := filepath.Dir(trashDir)                    // ".../Trash"
	infoDir := filepath.Join(parent, "info")            // ".../Trash/info"
	if err := os.MkdirAll(infoDir, 0700); err != nil { // ensure info dir
		return err
	}

	base := filepath.Base(trashedFilePath)              // e.g. "foo.txt"
	infoPath := filepath.Join(infoDir, base+".trashinfo")

	// The spec wants the absolute path of the original file.
	absOrig, _ := filepath.Abs(originalPath)

	// Current local time in ISO 8601 (no timezone, just local date/time).
	deletionTime := time.Now().Format("2006-01-02T15:04:05")

	content := fmt.Sprintf(
		`[Trash Info]
Path=%s
DeletionDate=%s
`, absOrig, deletionTime)

	// Write .trashinfo file
	f, err := os.Create(infoPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}

// Put moves 'path' into the XDG Put (on Linux) and creates
// a .trashinfo file. The final name of the trashed file may differ
// if a collision is found in the trash.
func Put(path string) (string, error) {
	// Ensure the file actually exists
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("trash: cannot trash non-existent file: %v", err)
	}

	trashDir, err := xdgTrashDir(path)
	if err != nil {
		return "", fmt.Errorf("trash: failed to get trash dir: %v", err)
	}

	// Create a unique name in the trash's "files" dir
	dest, err := uniqueTrashName(trashDir, path)
	if err != nil {
		return "", fmt.Errorf("trash: failed to determine unique trash name: %v", err)
	}

	// Attempt to rename (move) the file into the trash
	if err := os.Rename(path, dest); err != nil {
		return "", fmt.Errorf("trash: failed to move to trash: %v", err)
	}

	// Create the .trashinfo metadata
	if err := createTrashInfoFile(dest, path); err != nil {
		// If we fail to create trashinfo, we still have the file moved, but let's return error
		return dest, fmt.Errorf("trash: file trashed but failed to write .trashinfo: %v", err)
	}

	slog.Debug(fmt.Sprintf("trash: file moved to trash: %q", dest))
	return dest, nil
}
