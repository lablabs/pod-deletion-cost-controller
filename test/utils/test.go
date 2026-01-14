package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

func GetK8sBinaryDir(deep int, dirs []string) (string, error) {
	dirPath := filepath.Join(dirs...)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if deep <= 0 {
			return "", fmt.Errorf("not found")
		}
		parent := make([]string, 0)
		parent = append([]string{".."}, dirs...)
		return GetK8sBinaryDir(deep-1, parent)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(dirPath, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("not found")
}
