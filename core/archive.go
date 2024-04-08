package core

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (c *Core) RecoverArchive(did string, archivePath string) {
	// Source folder path
	c.log.Info("Recovering archive for DID", "DID", did)
	var source string
	if archivePath == "" {
		source = c.cfg.DirPath + "Backup" + "/" + did
	} else {
		source = archivePath + "/" + did
	}
	// Destination folder path
	destination := c.cfg.DirPath + RubixRootDir + did
	// Attempt to rename the folder
	if err := moveFolder(source, destination); err != nil {
		c.log.Error("Error recovering archive", "err", err)
		return
	}

	c.log.Info("Archive recovered successfully", "DID", did)
}
func (c *Core) Archive(did string, archivePath string) {
	// Source directory to be copied and zipped
	sourceDir := c.cfg.DirPath + RubixRootDir + did
	destinationDir := c.cfg.DirPath + "Backup" + "/" + did
	// zipFileName := did + ".zip"

	// Copy the source directory to the destination directory
	err := copyDir(sourceDir, destinationDir)
	if err != nil {
		c.log.Error("Error Backing up DID", "err", err)
		return
	}

	// Zip the copied directory
	// err = zipDir(destinationDir, zipFileName)
	// if err != nil {
	// 	fmt.Println("Error zipping directory:", err)
	// 	return
	// }

	c.log.Info("DID backed up successfully", "DID", did)
}

// copyDir copies a directory recursively
func copyDir(src, dst string) error {
	// Open source directory
	srcDir, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcDir.Close()

	err = os.MkdirAll(dst, 0777)
	if err != nil {
		fmt.Println("Error making directory:", err)
		return err
	}

	// Read the contents of the source directory
	fileInfos, err := srcDir.Readdir(-1)
	if err != nil {
		fmt.Printf("Failed to read contents of directory: %s", err)
		return err
	}

	for _, fileInfo := range fileInfos {
		sourcePath := filepath.Join(src, fileInfo.Name())
		destinationPath := filepath.Join(dst, fileInfo.Name())

		// If the current item is a directory, recursively copy it
		if fileInfo.IsDir() {
			err = copyDir(sourcePath, destinationPath)
			if err != nil {
				fmt.Println("Error copying directory:", err)
				return err
			}
		} else {
			// If the current item is a file, copy it
			err = copyFile(sourcePath, destinationPath)

			if err != nil {
				fmt.Println("Error copying file:", err)
				return err
			}
		}
	}

	return nil
}

// copyFile copies a file from source to destination
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

// zipDir zips a directory
func zipDir(sourceDir, zipFileName string) error {
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	err = filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the source directory itself
		if filePath == sourceDir {
			return nil
		}

		// Get the relative path of the file to be zipped
		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}

		// Create a zip entry header
		zipHeader, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		zipHeader.Name = relPath

		// Write the header to the zip archive
		writer, err := archive.CreateHeader(zipHeader)
		if err != nil {
			return err
		}

		// If the current item is not a directory, write its contents to the zip archive
		if !info.IsDir() {
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
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
