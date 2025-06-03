package trash

import (
	"fmt"
	"log"
)

func ExampleTrash() {
	// Move a file to trash
	err := Trash("/path/to/file.txt")
	if err != nil {
		log.Fatal(err)
	}

	// List items in trash
	items, err := List()
	if err != nil {
		log.Fatal(err)
	}

	for _, item := range items {
		fmt.Printf("Name: %s, Original: %s, Deleted: %s\n",
			item.Name, item.OriginalPath, item.DeletionDate.Format("2006-01-02 15:04:05"))
	}

	// Restore a file from trash
	if len(items) > 0 {
		err = Restore(items[0].Name)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Delete a specific item from trash permanently
	err = Delete("file.txt")
	if err != nil && err != ErrFileNotInTrash {
		log.Fatal(err)
	}

	// Empty entire trash
	err = Empty()
	if err != nil {
		log.Fatal(err)
	}
}
