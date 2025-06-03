package trash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrash(t *testing.T) {
	t.Run("TrashAndRestore", func(t *testing.T) {
		testFile := filepath.Join(t.TempDir(), "test.txt")
		content := []byte("test content")
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := Trash(testFile); err != nil {
			t.Fatalf("Failed to trash file: %v", err)
		}

		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Error("File still exists after trashing")
		}

		items, err := List()
		if err != nil {
			t.Fatalf("Failed to list trash: %v", err)
		}

		var found bool
		var itemName string
		for _, item := range items {
			if item.OriginalPath == testFile {
				found = true
				itemName = item.Name
				break
			}
		}

		if !found {
			t.Error("Trashed file not found in list")
		}

		if err := Restore(itemName); err != nil {
			t.Fatalf("Failed to restore file: %v", err)
		}

		restoredContent, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("Failed to read restored file: %v", err)
		}

		if string(restoredContent) != string(content) {
			t.Error("Restored file content doesn't match original")
		}
	})

	t.Run("WeirdFilenames", func(t *testing.T) {
		weirdNames := []string{
			"file with spaces.txt",
			"文件名.txt",
			"file\nwith\nnewlines.txt",
			"file\twith\ttabs.txt",
			"🚀emoji🎉file.txt",
			"file%20with%20percent.txt",
			"file?with*special<chars>.txt",
			"   leading_spaces.txt",
			"trailing_spaces.txt   ",
			".hidden_file",
			".",
			"..",
		}

		tempDir := t.TempDir()

		for _, name := range weirdNames {
			if name == "." || name == ".." {
				continue
			}

			testFile := filepath.Join(tempDir, name)
			if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
				t.Logf("Skipping unsupported filename: %s", name)
				continue
			}

			if err := Trash(testFile); err != nil {
				t.Errorf("Failed to trash file with weird name %q: %v", name, err)
				continue
			}

			if _, err := os.Stat(testFile); !os.IsNotExist(err) {
				t.Errorf("File %q still exists after trashing", name)
			}
		}
	})

	t.Run("Directory", func(t *testing.T) {
		testDir := filepath.Join(t.TempDir(), "test_dir")
		if err := os.MkdirAll(filepath.Join(testDir, "subdir"), 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		testFile := filepath.Join(testDir, "subdir", "file.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := Trash(testDir); err != nil {
			t.Fatalf("Failed to trash directory: %v", err)
		}

		if _, err := os.Stat(testDir); !os.IsNotExist(err) {
			t.Error("Directory still exists after trashing")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		testFile := filepath.Join(t.TempDir(), "delete_test.txt")
		if err := os.WriteFile(testFile, []byte("delete me"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := Trash(testFile); err != nil {
			t.Fatalf("Failed to trash file: %v", err)
		}

		items, err := List()
		if err != nil {
			t.Fatalf("Failed to list trash: %v", err)
		}

		var itemName string
		for _, item := range items {
			if item.OriginalPath == testFile {
				itemName = item.Name
				break
			}
		}

		if itemName == "" {
			t.Fatal("Trashed file not found")
		}

		if err := Delete(itemName); err != nil {
			t.Fatalf("Failed to delete from trash: %v", err)
		}

		items, err = List()
		if err != nil {
			t.Fatalf("Failed to list trash after delete: %v", err)
		}

		for _, item := range items {
			if item.OriginalPath == testFile {
				t.Error("File still in trash after delete")
			}
		}
	})

	t.Run("SymbolicLinks", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a target file
		targetFile := filepath.Join(tempDir, "target.txt")
		if err := os.WriteFile(targetFile, []byte("target content"), 0644); err != nil {
			t.Fatalf("Failed to create target file: %v", err)
		}

		// Create a symbolic link
		linkFile := filepath.Join(tempDir, "link.txt")
		if err := os.Symlink(targetFile, linkFile); err != nil {
			t.Skipf("Failed to create symlink (may not be supported): %v", err)
		}

		// Verify the link exists and points to the right place
		linkTarget, err := os.Readlink(linkFile)
		if err != nil {
			t.Fatalf("Failed to read symlink: %v", err)
		}
		if linkTarget != targetFile {
			t.Errorf("Link target mismatch: got %s, want %s", linkTarget, targetFile)
		}

		// Trash the symbolic link
		if err := Trash(linkFile); err != nil {
			t.Fatalf("Failed to trash symlink: %v", err)
		}

		// Verify link is gone but target still exists
		if _, err := os.Lstat(linkFile); !os.IsNotExist(err) {
			t.Error("Symlink still exists after trashing")
		}
		if _, err := os.Stat(targetFile); err != nil {
			t.Error("Target file was removed when trashing symlink")
		}

		// Find the trashed link
		items, err := List()
		if err != nil {
			t.Fatalf("Failed to list trash: %v", err)
		}

		var linkItem *TrashItem
		for _, item := range items {
			if item.OriginalPath == linkFile {
				linkItem = &item
				break
			}
		}

		if linkItem == nil {
			t.Fatal("Trashed symlink not found in trash")
		}

		// Check that the trashed item is still a symlink
		info, err := os.Lstat(linkItem.FilePath)
		if err != nil {
			t.Fatalf("Failed to stat trashed link: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("Trashed item is not a symlink")
		}

		// Restore the symlink
		if err := Restore(linkItem.Name); err != nil {
			t.Fatalf("Failed to restore symlink: %v", err)
		}

		// Verify the restored link
		restoredTarget, err := os.Readlink(linkFile)
		if err != nil {
			t.Fatalf("Failed to read restored symlink: %v", err)
		}
		if restoredTarget != targetFile {
			t.Errorf("Restored link target mismatch: got %s, want %s", restoredTarget, targetFile)
		}

		// Verify we can still access the target through the link
		content, err := os.ReadFile(linkFile)
		if err != nil {
			t.Fatalf("Failed to read through restored symlink: %v", err)
		}
		if string(content) != "target content" {
			t.Error("Content mismatch when reading through restored symlink")
		}
	})

	t.Run("RestoreConflict", func(t *testing.T) {
		testFile := filepath.Join(t.TempDir(), "conflict.txt")
		if err := os.WriteFile(testFile, []byte("original"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := Trash(testFile); err != nil {
			t.Fatalf("Failed to trash file: %v", err)
		}

		if err := os.WriteFile(testFile, []byte("new file"), 0644); err != nil {
			t.Fatalf("Failed to create conflicting file: %v", err)
		}

		items, err := List()
		if err != nil {
			t.Fatalf("Failed to list trash: %v", err)
		}

		var itemName string
		for _, item := range items {
			if item.OriginalPath == testFile {
				itemName = item.Name
				break
			}
		}

		if err := Restore(itemName); err != ErrAlreadyExists {
			t.Errorf("Expected ErrAlreadyExists, got: %v", err)
		}
	})
}

func TestTrashNameGeneration(t *testing.T) {
	tempDir := t.TempDir()

	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tempDir, "duplicate.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := Trash(testFile); err != nil {
			t.Fatalf("Failed to trash file %d: %v", i, err)
		}
	}

	items, err := List()
	if err != nil {
		t.Fatalf("Failed to list trash: %v", err)
	}

	duplicateCount := 0
	for _, item := range items {
		if filepath.Base(item.OriginalPath) == "duplicate.txt" &&
			filepath.Dir(item.OriginalPath) == tempDir {
			duplicateCount++
		}
	}

	if duplicateCount != 5 {
		t.Errorf("Expected 5 duplicate files in trash, got %d", duplicateCount)
	}
}
