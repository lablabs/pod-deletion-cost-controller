package utils

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

func GetK8sBinaryDir(deep int, dirs []string) (string, error) {
	dirPath := path.Join(dirs...)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if deep <= 0 {
			return "", fmt.Errorf("not found")
		}
		parent := make([]string, 0)
		parent = append(parent, "..")
		parent = append(parent, dirPath)
		return GetK8sBinaryDir(deep-1, parent)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(dirPath, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("not found")
}
