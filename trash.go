package trash

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrTrashNotFound     = errors.New("trash directory not found")
	ErrInvalidTrashInfo  = errors.New("invalid trash info file")
	ErrFileNotInTrash    = errors.New("file not found in trash")
	ErrRestoreFailed     = errors.New("restore operation failed")
	ErrAlreadyExists     = errors.New("file already exists at destination")
	ErrCrossDevice       = errors.New("cannot move across devices")
	ErrNoTrashAvailable  = errors.New("no trash directory available")
)

type TrashItem struct {
	Name         string
	OriginalPath string
	DeletionDate time.Time
	InfoPath     string
	FilePath     string
	TrashDir     string
}

var (
	homeTrash string
	uid       string
	initOnce  sync.Once
	initErr   error
)

func initialize() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		initErr = fmt.Errorf("failed to get home directory: %w", err)
		return
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(homeDir, ".local", "share")
	}

	homeTrash = filepath.Join(dataHome, "Trash")
	
	if err := ensureTrashDirs(homeTrash); err != nil {
		initErr = err
		return
	}

	currentUser, err := user.Current()
	if err != nil {
		initErr = fmt.Errorf("failed to get current user: %w", err)
		return
	}

	uid = currentUser.Uid
}

func ensureInitialized() error {
	initOnce.Do(initialize)
	return initErr
}

func ensureTrashDirs(trashDir string) error {
	dirs := []string{
		filepath.Join(trashDir, "files"),
		filepath.Join(trashDir, "info"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create trash directory %s: %w", dir, err)
		}
	}

	return nil
}

func Trash(path string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	trashDir, err := getTrashDirForPath(absPath)
	if err != nil {
		return fmt.Errorf("failed to determine trash directory: %w", err)
	}

	if err := ensureTrashDirs(trashDir); err != nil {
		return fmt.Errorf("failed to create trash directories: %w", err)
	}

	baseName := filepath.Base(absPath)
	trashName := generateTrashNameInDir(baseName, trashDir)
	
	filesPath := filepath.Join(trashDir, "files", trashName)
	infoPath := filepath.Join(trashDir, "info", trashName+".trashinfo")

	if err := writeTrashInfo(infoPath, absPath, time.Now()); err != nil {
		return fmt.Errorf("failed to write trash info: %w", err)
	}

	if err := moveToTrash(absPath, filesPath, info); err != nil {
		os.Remove(infoPath)
		return fmt.Errorf("failed to move to trash: %w", err)
	}

	return nil
}

func generateTrashName(baseName string) string {
	return generateTrashNameInDir(baseName, homeTrash)
}

func generateTrashNameInDir(baseName string, trashDir string) string {
	baseName = sanitizeFilename(baseName)
	
	for i := 0; i < 100; i++ {
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s.%d", baseName, i)
		}
		
		filesPath := filepath.Join(trashDir, "files", name)
		infoPath := filepath.Join(trashDir, "info", name+".trashinfo")
		
		if _, err := os.Lstat(filesPath); os.IsNotExist(err) {
			if _, err := os.Lstat(infoPath); os.IsNotExist(err) {
				return name
			}
		}
	}
	
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	return fmt.Sprintf("%s.%s", baseName, hex.EncodeToString(randomBytes))
}

func sanitizeFilename(name string) string {
	if name == "" {
		return "unnamed"
	}
	
	name = strings.TrimSpace(name)
	
	if strings.HasPrefix(name, ".") && len(name) == 1 {
		return "dot"
	}
	
	return name
}

func writeTrashInfo(infoPath, originalPath string, deletionTime time.Time) error {
	encodedPath := url.QueryEscape(originalPath)
	encodedPath = strings.ReplaceAll(encodedPath, "+", "%20")
	
	content := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		encodedPath,
		deletionTime.UTC().Format("2006-01-02T15:04:05"))
	
	return os.WriteFile(infoPath, []byte(content), 0600)
}

func moveToTrash(src, dst string, info os.FileInfo) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	
	if !isCrossDeviceError(err) {
		return err
	}
	
	if info.IsDir() {
		return copyDirAcrossDevices(src, dst)
	}
	
	return copyFileAcrossDevices(src, dst, info)
}

func copyFileAcrossDevices(src, dst string, info os.FileInfo) error {
	// Handle symbolic links specially
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}
		
		if err := os.Symlink(link, dst); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
		
		// Note: os.Chtimes doesn't work on symlinks on most systems
		// The symlink will have the current time as its modification time
		
		return os.Remove(src)
	}
	
	// Regular file handling
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()
	
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return err
	}
	
	if err := dstFile.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	
	if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
		os.Remove(dst)
		return err
	}
	
	return os.Remove(src)
}

func copyDirAcrossDevices(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		
		info, err := entry.Info()
		if err != nil {
			return err
		}
		
		// Check if it's a symlink before checking if it's a directory
		// because symlinks to directories would return true for IsDir()
		if info.Mode()&os.ModeSymlink != 0 {
			if err := copyFileAcrossDevices(srcPath, dstPath, info); err != nil {
				return err
			}
		} else if entry.IsDir() {
			if err := copyDirAcrossDevices(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFileAcrossDevices(srcPath, dstPath, info); err != nil {
				return err
			}
		}
	}
	
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	
	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		return err
	}
	
	return os.RemoveAll(src)
}

func List() ([]TrashItem, error) {
	if err := ensureInitialized(); err != nil {
		return nil, err
	}
	var items []TrashItem
	
	// List items from home trash
	homeItems, err := listTrashDir(homeTrash)
	if err == nil {
		items = append(items, homeItems...)
	}
	
	// List items from all mounted filesystems
	mountPoints, err := getMountPoints()
	if err == nil {
		for _, mount := range mountPoints {
			if mount == "/" {
				continue // Already handled by home trash
			}
			
			trashDir := filepath.Join(mount, ".Trash-"+uid)
			if info, err := os.Stat(trashDir); err == nil && info.IsDir() {
				mountItems, err := listTrashDir(trashDir)
				if err == nil {
					items = append(items, mountItems...)
				}
			}
		}
	}
	
	return items, nil
}

func listTrashDir(trashDir string) ([]TrashItem, error) {
	infoDir := filepath.Join(trashDir, "info")
	entries, err := os.ReadDir(infoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TrashItem{}, nil
		}
		return nil, fmt.Errorf("failed to read info directory: %w", err)
	}
	
	var items []TrashItem
	
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".trashinfo") {
			continue
		}
		
		infoPath := filepath.Join(infoDir, entry.Name())
		item, err := parseTrashInfo(infoPath, trashDir)
		if err != nil {
			continue
		}
		
		items = append(items, item)
	}
	
	return items, nil
}

func parseTrashInfo(infoPath string, trashDir string) (TrashItem, error) {
	content, err := os.ReadFile(infoPath)
	if err != nil {
		return TrashItem{}, err
	}
	
	lines := strings.Split(string(content), "\n")
	if len(lines) < 3 || lines[0] != "[Trash Info]" {
		return TrashItem{}, ErrInvalidTrashInfo
	}
	
	var originalPath string
	var deletionDate time.Time
	
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "Path=") {
			pathStr := strings.TrimPrefix(line, "Path=")
			originalPath, _ = url.QueryUnescape(pathStr)
		} else if strings.HasPrefix(line, "DeletionDate=") {
			dateStr := strings.TrimPrefix(line, "DeletionDate=")
			deletionDate, _ = time.Parse("2006-01-02T15:04:05", dateStr)
		}
	}
	
	if originalPath == "" {
		return TrashItem{}, ErrInvalidTrashInfo
	}
	
	baseName := strings.TrimSuffix(filepath.Base(infoPath), ".trashinfo")
	
	return TrashItem{
		Name:         baseName,
		OriginalPath: originalPath,
		DeletionDate: deletionDate,
		InfoPath:     infoPath,
		FilePath:     filepath.Join(trashDir, "files", baseName),
		TrashDir:     trashDir,
	}, nil
}

func Restore(trashName string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	item, err := findTrashItem(trashName)
	if err != nil {
		return err
	}
	
	if _, err := os.Lstat(item.OriginalPath); err == nil {
		return ErrAlreadyExists
	}
	
	dir := filepath.Dir(item.OriginalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}
	
	if err := os.Rename(item.FilePath, item.OriginalPath); err != nil {
		return fmt.Errorf("failed to restore file: %w", err)
	}
	
	if err := os.Remove(item.InfoPath); err != nil {
		os.Rename(item.OriginalPath, item.FilePath)
		return fmt.Errorf("failed to remove info file: %w", err)
	}
	
	return nil
}

func findTrashItem(trashName string) (TrashItem, error) {
	// Check home trash first
	infoPath := filepath.Join(homeTrash, "info", trashName+".trashinfo")
	if _, err := os.Stat(infoPath); err == nil {
		return parseTrashInfo(infoPath, homeTrash)
	}
	
	// Check all mounted filesystems
	mountPoints, err := getMountPoints()
	if err == nil {
		for _, mount := range mountPoints {
			if mount == "/" {
				continue
			}
			
			trashDir := filepath.Join(mount, ".Trash-"+uid)
			infoPath := filepath.Join(trashDir, "info", trashName+".trashinfo")
			if _, err := os.Stat(infoPath); err == nil {
				return parseTrashInfo(infoPath, trashDir)
			}
		}
	}
	
	return TrashItem{}, ErrFileNotInTrash
}

func Empty() error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	// Empty home trash
	if err := emptyTrashDir(homeTrash); err != nil {
		return err
	}
	
	// Empty trash on all mounted filesystems
	mountPoints, err := getMountPoints()
	if err == nil {
		for _, mount := range mountPoints {
			if mount == "/" {
				continue
			}
			
			trashDir := filepath.Join(mount, ".Trash-"+uid)
			if info, err := os.Stat(trashDir); err == nil && info.IsDir() {
				if err := emptyTrashDir(trashDir); err != nil {
					return err
				}
			}
		}
	}
	
	return nil
}

func emptyTrashDir(trashDir string) error {
	filesDir := filepath.Join(trashDir, "files")
	infoDir := filepath.Join(trashDir, "info")
	
	if err := emptyDir(filesDir); err != nil {
		return fmt.Errorf("failed to empty files directory: %w", err)
	}
	
	if err := emptyDir(infoDir); err != nil {
		return fmt.Errorf("failed to empty info directory: %w", err)
	}
	
	return nil
}

func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	
	return nil
}

func Delete(trashName string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	item, err := findTrashItem(trashName)
	if err != nil {
		return err
	}
	
	if err := os.RemoveAll(item.FilePath); err != nil {
		return fmt.Errorf("failed to remove file: %w", err)
	}
	
	if err := os.Remove(item.InfoPath); err != nil {
		return fmt.Errorf("failed to remove info file: %w", err)
	}
	
	return nil
}

func getTrashDirForPath(path string) (string, error) {
	pathMount, err := getMountPoint(path)
	if err != nil {
		return "", err
	}
	
	homeMount, err := getMountPoint(homeTrash)
	if err != nil {
		return "", err
	}
	
	// If on same filesystem as home, use home trash
	if pathMount == homeMount {
		return homeTrash, nil
	}
	
	// Otherwise, use .Trash-$uid on the mount point
	trashDir := filepath.Join(pathMount, ".Trash-"+uid)
	
	// Check if we can create/use this trash directory
	if err := checkTrashDirSecurity(trashDir); err != nil {
		// If we can't use the trash dir on this mount, fall back to home trash
		// This may result in cross-device moves, but it's better than failing
		return homeTrash, nil
	}
	
	return trashDir, nil
}

func checkTrashDirSecurity(trashDir string) error {
	info, err := os.Stat(trashDir)
	if os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(trashDir, 0700); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	
	// Check that it's a directory
	if !info.IsDir() {
		return fmt.Errorf("trash path exists but is not a directory")
	}
	
	// Check permissions (should be 0700)
	if info.Mode().Perm() != 0700 {
		return fmt.Errorf("trash directory has incorrect permissions")
	}
	
	return nil
}
