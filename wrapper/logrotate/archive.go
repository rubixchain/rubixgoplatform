package logrotate

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
)

// compress writes srcPath into a ZIP archive at zipPath, storing it under
// entryName.
//
// The archive is built as a temporary file and renamed into place only after a
// successful compression, so a partial or corrupt archive is never left behind
// and the source log file is never touched by a failure.
func compress(srcPath string, entryName string, zipPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open rotated log file %v, err: %v", srcPath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat rotated log file %v, err: %v", srcPath, err)
	}

	tmpPath := zipPath + ".tmp"
	if err := writeZip(tmpPath, entryName, info, src); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, zipPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to move log archive to %v, err: %v", zipPath, err)
	}

	return nil
}

func writeZip(tmpPath string, entryName string, info os.FileInfo, src io.Reader) error {
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, logFilePerm)
	if err != nil {
		return fmt.Errorf("failed to create log archive %v, err: %v", tmpPath, err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("failed to build log archive header, err: %v", err)
	}
	header.Name = entryName
	header.Method = zip.Deflate

	entry, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create log archive entry %v, err: %v", entryName, err)
	}

	if _, err := io.Copy(entry, src); err != nil {
		return fmt.Errorf("failed to compress log file into %v, err: %v", tmpPath, err)
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed to finalise log archive %v, err: %v", tmpPath, err)
	}

	if err := out.Sync(); err != nil {
		return fmt.Errorf("failed to flush log archive %v, err: %v", tmpPath, err)
	}

	return nil
}
