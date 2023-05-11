package utils

import (
	"os"
	"path/filepath"
)

// SearchUp finds fileName somewhere in directory of relativeTo or in any of its parent directories.
func SearchUp(fileName string, relativeTo string) (xs string, xb bool, xe error) {
	curDir := filepath.Dir(relativeTo)

	for {
		path := filepath.Join(curDir, fileName)

		// Check if file exists and is not a directory
		fileInfo, err := os.Stat(path)
		if err == nil && !fileInfo.IsDir() {
			return path, true, nil
		}

		// Ignore permission errors
		if err != nil && !os.IsPermission(err) && !os.IsNotExist(err) {
			return "", false, err
		}

		parentDir := filepath.Dir(curDir)
		if parentDir == curDir {
			break
		}

		curDir = parentDir
	}

	return "", false, nil
}
