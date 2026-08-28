package sqliteutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func Prepare(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("SQLite 数据库路径不能为空")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if directory != "." {
		if err := os.Chmod(directory, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func Secure(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
