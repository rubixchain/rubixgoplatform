package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type RecoverArchiveReq struct {
	Did string `json:"did"`
}

func (c *Core) RecoverArchive(did string, archivePath string) {
	// Source folder path
	fmt.Println("Did:", did)
	source := archivePath + "/" + did
	fmt.Println("Source:", source)
	fmt.Println("Archive Path:", archivePath)

	// Destination folder path
	destination := c.cfg.DirPath + RubixRootDir + did
	fmt.Println("Destination:", destination)
	// Attempt to rename the folder
	if err := moveFolder(source, destination); err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Folder and its contents moved successfully!")
}

func moveFolder(source, destination string) error {
	// Create destination folder if it doesn't exist
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}

	// Walk through the source directory
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Construct the corresponding path in the destination directory
		destPath := filepath.Join(destination, path[len(source):])

		// If it's a directory, create it in the destination
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Otherwise, move the file to the destination
		return moveFile(path, destPath)
	})

	if err != nil {
		return err
	}

	// Remove the source directory after successfully moving its content
	return os.RemoveAll(source)
}

func moveFile(source, destination string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	// Remove the source file after successful copy
	return os.Remove(source)
}
